package inference

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/heihei0299/pi-switch/internal/domain/usage"
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type OutboundRequest struct {
	Request *http.Request
	Client  HTTPDoer
	URL     string
}

type OutboundRequestBuilder func([]byte) (OutboundRequest, error)

type NonStreamOutcomeKind uint8

const (
	NonStreamOutcomeError NonStreamOutcomeKind = iota + 1
	NonStreamOutcomeUpstreamResponse
	NonStreamOutcomeSuccess
	NonStreamOutcomeCanceled
)

type NonStreamOutcome struct {
	Kind      NonStreamOutcomeKind
	Status    int
	Headers   http.Header
	Body      []byte
	URL       string
	Error     string
	ErrorType string
	ShouldLog bool
	Usage     *usage.UsageSummary
	Cost      *float64
}

func ExecuteNonStream(ctx context.Context, prepared PreparedRequest, build OutboundRequestBuilder) NonStreamOutcome {
	body, _ := json.Marshal(prepared.Body)
	outbound, err := build(body)
	if err != nil {
		return NonStreamOutcome{Kind: NonStreamOutcomeError, Status: http.StatusBadGateway, Error: err.Error(), ErrorType: "upstream_error"}
	}
	response, err := outbound.Client.Do(outbound.Request)
	if err != nil {
		return transportFailure(ctx, err, outbound.URL)
	}
	responseBody, err := readResponse(ctx, response)
	if err != nil {
		return transportFailure(ctx, err, outbound.URL)
	}
	if response.StatusCode >= http.StatusBadRequest {
		if response.StatusCode == http.StatusBadRequest && strings.Contains(string(responseBody), "invalid_request_error") {
			if retry, handled := retryNonStream(ctx, prepared, build); handled {
				return retry
			}
		}
		return NonStreamOutcome{
			Kind: NonStreamOutcomeUpstreamResponse, Status: response.StatusCode, Headers: response.Header, Body: responseBody,
			URL: outbound.URL, Error: string(responseBody), ShouldLog: true,
		}
	}
	return processSuccessfulResponse(prepared, response, responseBody, outbound.URL)
}

func retryNonStream(ctx context.Context, prepared PreparedRequest, build OutboundRequestBuilder) (NonStreamOutcome, bool) {
	body, err := prepared.MinTokenRetryBody()
	if err != nil {
		return NonStreamOutcome{}, false
	}
	encoded, _ := json.Marshal(body)
	outbound, err := build(encoded)
	if err != nil {
		return NonStreamOutcome{}, false
	}
	response, err := outbound.Client.Do(outbound.Request)
	if err != nil {
		if ctx.Err() != nil {
			return transportFailure(ctx, err, outbound.URL), true
		}
		return NonStreamOutcome{}, false
	}
	responseBody, err := readResponse(ctx, response)
	if err != nil {
		return transportFailure(ctx, err, outbound.URL), true
	}
	if response.StatusCode >= http.StatusBadRequest {
		return NonStreamOutcome{}, false
	}
	return processSuccessfulResponse(prepared, response, responseBody, outbound.URL), true
}

func processSuccessfulResponse(prepared PreparedRequest, response *http.Response, body []byte, upstreamURL string) NonStreamOutcome {
	result, err := processNonStreamResponse(prepared, body)
	if err != nil {
		return NonStreamOutcome{
			Kind: NonStreamOutcomeError, Status: http.StatusBadGateway, Error: err.Error(), ErrorType: "conversion_error",
			URL: upstreamURL, ShouldLog: true,
		}
	}
	return NonStreamOutcome{
		Kind: NonStreamOutcomeSuccess, Status: response.StatusCode, Headers: response.Header, Body: result.Body,
		URL: upstreamURL, ShouldLog: true, Usage: result.Usage, Cost: result.Cost,
	}
}

func transportFailure(ctx context.Context, err error, upstreamURL string) NonStreamOutcome {
	status := http.StatusBadGateway
	noResponse := false
	if ctx.Err() != nil {
		status, noResponse = 499, true
	}
	kind := NonStreamOutcomeError
	if noResponse {
		kind = NonStreamOutcomeCanceled
	}
	return NonStreamOutcome{
		Kind: kind, Status: status, Error: err.Error(), ErrorType: "upstream_error", URL: upstreamURL,
		ShouldLog: true,
	}
}

func readResponse(ctx context.Context, response *http.Response) ([]byte, error) {
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if ctxErr := ctx.Err(); ctxErr != nil {
		err = ctxErr
	}
	return body, err
}
