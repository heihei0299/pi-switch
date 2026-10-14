package inference

import (
	"context"
	"io"
	"net/http"
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
	builder := func([]byte) (OutboundRequest, error) {
		return testOutbound(func(context.Context) (OutboundResponse, error) {
			response := testResponse(http.StatusPartialContent, "data: [DONE]\n\n")
			response.URL = "http://upstream.test/stream"
			return response, nil
		}), nil
	}
	writer := newRelayTestRecorder()

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
	builder := func([]byte) (OutboundRequest, error) {
		return testOutbound(func(context.Context) (OutboundResponse, error) {
			response := testResponse(http.StatusTooManyRequests, strings.Repeat("x", streamErrorBodyLimit+1))
			response.Headers["X-Upstream"] = []string{"kept"}
			response.URL = "http://upstream.test/stream"
			return response, nil
		}), nil
	}

	result := ExecuteStream(context.Background(), prepared, builder, newRelayTestRecorder())
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
	builder := func([]byte) (OutboundRequest, error) {
		return testOutbound(func(context.Context) (OutboundResponse, error) {
			cancel()
			return OutboundResponse{URL: "http://upstream.test/stream"}, context.Canceled
		}), nil
	}

	result := ExecuteStream(ctx, prepared, builder, newRelayTestRecorder())
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
		return nil, io.ErrUnexpectedEOF
	}, newRelayTestRecorder())
	if result.Kind != StreamOutcomeError || result.ErrorType != "upstream_error" || result.ShouldLog {
		t.Fatalf("outcome = %+v, want unlogged build error", result)
	}
}
