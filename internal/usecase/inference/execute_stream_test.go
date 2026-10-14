package inference

import (
	"context"
	"io"
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
			response := testResponse(206, "data: [DONE]\n\n")
			response.URL = "http://upstream.test/stream"
			return response, nil
		}), nil
	}
	writer := newRelayTestSink()
	var started struct {
		status  int
		headers Header
	}

	result := ExecuteStream(context.Background(), prepared, builder, func(status int, headers Header) {
		started.status, started.headers = status, headers
	}, writer)
	if result.Kind != StreamOutcomeRelayed || result.Facts.Status != 206 || started.status != 206 || started.headers.Get("Content-Type") != "application/json" {
		t.Fatalf("outcome = %+v, response status = %d; want relayed 206", result, started.status)
	}
	if writer.String() != "data: [DONE]\n\n" || result.URL != "http://upstream.test/stream" {
		t.Fatalf("body/url = %q/%q", writer.String(), result.URL)
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
			response := testResponse(429, strings.Repeat("x", streamErrorBodyLimit+1))
			response.Headers["X-Upstream"] = []string{"kept"}
			response.URL = "http://upstream.test/stream"
			return response, nil
		}), nil
	}

	result := ExecuteStream(context.Background(), prepared, builder, nil, newRelayTestSink())
	if result.Kind != StreamOutcomeUpstreamResponse || result.Facts.Status != 429 || len(result.Body) != streamErrorBodyLimit || result.Headers.Get("X-Upstream") != "kept" {
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

	result := ExecuteStream(ctx, prepared, builder, nil, newRelayTestSink())
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
	}, nil, newRelayTestSink())
	if result.Kind != StreamOutcomeError || result.ErrorType != "upstream_error" || result.ShouldLog {
		t.Fatalf("outcome = %+v, want unlogged build error", result)
	}
}
