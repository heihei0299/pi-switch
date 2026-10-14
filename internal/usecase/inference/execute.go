package inference

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/heihei0299/pi-switch/internal/domain/usage"
)

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
	Headers   Header
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
		return NonStreamOutcome{Kind: NonStreamOutcomeError, Status: 502, Error: err.Error(), ErrorType: "upstream_error"}
	}
	response, err := outbound.Do(ctx)
	if err != nil {
		return transportFailure(ctx, err, response.URL)
	}
	responseBody, err := readResponse(ctx, response)
	if err != nil {
		return transportFailure(ctx, err, response.URL)
	}
	if response.Status >= 400 {
		if response.Status == 400 && strings.Contains(string(responseBody), "invalid_request_error") {
			if retry, handled := retryNonStream(ctx, prepared, build); handled {
				return retry
			}
		}
		return NonStreamOutcome{
			Kind: NonStreamOutcomeUpstreamResponse, Status: response.Status, Headers: response.Headers, Body: responseBody,
			URL: response.URL, Error: string(responseBody), ShouldLog: true,
		}
	}
	return processSuccessfulResponse(prepared, response, responseBody)
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
	response, err := outbound.Do(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return transportFailure(ctx, err, response.URL), true
		}
		return NonStreamOutcome{}, false
	}
	responseBody, err := readResponse(ctx, response)
	if err != nil {
		return transportFailure(ctx, err, response.URL), true
	}
	if response.Status >= 400 {
		return NonStreamOutcome{}, false
	}
	return processSuccessfulResponse(prepared, response, responseBody), true
}

func processSuccessfulResponse(prepared PreparedRequest, response OutboundResponse, body []byte) NonStreamOutcome {
	result, err := processNonStreamResponse(prepared, body)
	if err != nil {
		return NonStreamOutcome{
			Kind: NonStreamOutcomeError, Status: 502, Error: err.Error(), ErrorType: "conversion_error",
			URL: response.URL, ShouldLog: true,
		}
	}
	return NonStreamOutcome{
		Kind: NonStreamOutcomeSuccess, Status: response.Status, Headers: response.Headers, Body: result.Body,
		URL: response.URL, ShouldLog: true, Usage: result.Usage, Cost: result.Cost,
	}
}

func transportFailure(ctx context.Context, err error, upstreamURL string) NonStreamOutcome {
	status := 502
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

func readResponse(ctx context.Context, response OutboundResponse) ([]byte, error) {
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if ctxErr := ctx.Err(); ctxErr != nil {
		err = ctxErr
	}
	return body, err
}
