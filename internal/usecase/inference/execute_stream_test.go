package inference

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/domain/protocol"
)

func TestExecuteStreamRelaysWithUpstreamStatus(t *testing.T) {
	plan, err := protocol.PlanRequest("chat", protocol.OpenAIChat, "auto")
	if err != nil {
		t.Fatal(err)
	}
	prepared := PreparedRequest{Model: "model", ProtocolPlan: plan, Body: map[string]interface{}{"model": "model"}}
	builder := func(body []byte) (OutboundRequest, error) {
		request, err := http.NewRequest(http.MethodPost, "http://upstream.test", bytes.NewReader(body))
		if err != nil {
			return OutboundRequest{}, err
		}
		return OutboundRequest{Request: request, URL: "http://upstream.test/stream", Client: testDoer(func(*http.Request) (*http.Response, error) {
			return testResponse(http.StatusPartialContent, "data: [DONE]\n\n"), nil
		})}, nil
	}
	writer := httptest.NewRecorder()

	result := ExecuteStream(context.Background(), prepared, builder, writer)
	if result.Kind != StreamOutcomeRelayed || result.Facts.Status != http.StatusPartialContent || writer.Code != http.StatusPartialContent {
		t.Fatalf("outcome = %+v, response status = %d; want relayed 206", result, writer.Code)
	}
	if writer.Body.String() != "data: [DONE]\n\n" || result.URL != "http://upstream.test/stream" {
		t.Fatalf("body/url = %q/%q", writer.Body.String(), result.URL)
	}
}

func TestExecuteStreamPreservesBoundedUpstreamErrorResponse(t *testing.T) {
	plan, err := protocol.PlanRequest("chat", protocol.OpenAIChat, "auto")
	if err != nil {
		t.Fatal(err)
	}
	prepared := PreparedRequest{Model: "model", ProtocolPlan: plan, Body: map[string]interface{}{"model": "model"}}
	builder := func(body []byte) (OutboundRequest, error) {
		request, err := http.NewRequest(http.MethodPost, "http://upstream.test", bytes.NewReader(body))
		if err != nil {
			return OutboundRequest{}, err
		}
		response := testResponse(http.StatusTooManyRequests, strings.Repeat("x", streamErrorBodyLimit+1))
		response.Header.Set("X-Upstream", "kept")
		return OutboundRequest{Request: request, URL: "http://upstream.test/stream", Client: testDoer(func(*http.Request) (*http.Response, error) { return response, nil })}, nil
	}

	result := ExecuteStream(context.Background(), prepared, builder, httptest.NewRecorder())
	if result.Kind != StreamOutcomeUpstreamResponse || result.Facts.Status != http.StatusTooManyRequests || len(result.Body) != streamErrorBodyLimit || result.Headers.Get("X-Upstream") != "kept" {
		t.Fatalf("outcome kind/status/body/header = %v/%d/%d/%q", result.Kind, result.Facts.Status, len(result.Body), result.Headers.Get("X-Upstream"))
	}
}

func TestExecuteStreamCanceledRequestReturnsNoResponse(t *testing.T) {
	plan, err := protocol.PlanRequest("chat", protocol.OpenAIChat, "auto")
	if err != nil {
		t.Fatal(err)
	}
	prepared := PreparedRequest{Model: "model", ProtocolPlan: plan, Body: map[string]interface{}{"model": "model"}}
	ctx, cancel := context.WithCancel(context.Background())
	builder := func(body []byte) (OutboundRequest, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://upstream.test", bytes.NewReader(body))
		if err != nil {
			return OutboundRequest{}, err
		}
		return OutboundRequest{Request: request, URL: "http://upstream.test/stream", Client: testDoer(func(*http.Request) (*http.Response, error) {
			cancel()
			return nil, context.Canceled
		})}, nil
	}

	result := ExecuteStream(ctx, prepared, builder, httptest.NewRecorder())
	if result.Kind != StreamOutcomeCanceled || result.Facts.Status != 499 || !result.ShouldLog {
		t.Fatalf("outcome = %+v, want logged cancellation without response", result)
	}
}

func TestExecuteStreamBuildFailureIsNotLogged(t *testing.T) {
	plan, err := protocol.PlanRequest("chat", protocol.OpenAIChat, "auto")
	if err != nil {
		t.Fatal(err)
	}
	prepared := PreparedRequest{Model: "model", ProtocolPlan: plan, Body: map[string]interface{}{"model": "model"}}
	result := ExecuteStream(context.Background(), prepared, func([]byte) (OutboundRequest, error) {
		return OutboundRequest{}, io.ErrUnexpectedEOF
	}, httptest.NewRecorder())
	if result.Kind != StreamOutcomeError || result.ErrorType != "upstream_error" || result.ShouldLog {
		t.Fatalf("outcome = %+v, want unlogged build error", result)
	}
}
