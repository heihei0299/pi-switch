package inference

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/domain/protocol"
)

type testOutbound func(context.Context) (OutboundResponse, error)

func (f testOutbound) Do(ctx context.Context) (OutboundResponse, error) { return f(ctx) }

func testResponse(status int, body string) OutboundResponse {
	return OutboundResponse{Status: status, Headers: Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestExecuteNonStreamRetriesInvalidRequestAndUsesRetrySuccess(t *testing.T) {
	plan, err := protocol.PlanRequest("chat", protocol.OpenAIChat, "auto")
	if err != nil {
		t.Fatal(err)
	}
	prepared := PreparedRequest{
		Supplier: "supplier", Model: "model", ProtocolPlan: plan,
		Body:       map[string]interface{}{"model": "model", "messages": []interface{}{}},
		sourceBody: map[string]interface{}{"model": "model", "messages": []interface{}{}},
	}
	ctx := context.Background()
	var bodies [][]byte
	calls := 0
	builder := func(body []byte) (OutboundRequest, error) {
		bodies = append(bodies, append([]byte(nil), body...))
		return testOutbound(func(context.Context) (OutboundResponse, error) {
			calls++
			if calls == 1 {
				response := testResponse(http.StatusBadRequest, `{"type":"invalid_request_error"}`)
				response.URL = "http://upstream.test/v1/chat/completions"
				return response, nil
			}
			response := testResponse(http.StatusOK, `{"id":"ok","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":2}}`)
			response.URL = "http://upstream.test/v1/chat/completions"
			return response, nil
		}), nil
	}

	result := ExecuteNonStream(ctx, prepared, builder)
	if calls != 2 || result.Kind != NonStreamOutcomeSuccess || result.Status != http.StatusOK || !result.ShouldLog {
		t.Fatalf("outcome = %+v, calls=%d; want successful retry", result, calls)
	}
	if len(bodies) != 2 {
		t.Fatalf("built request count = %d, want 2", len(bodies))
	}
	var retryBody map[string]interface{}
	if err := json.Unmarshal(bodies[1], &retryBody); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"max_tokens", "max_output_tokens", "max_completion_tokens"} {
		if retryBody[key] != float64(16) {
			t.Errorf("retry %s = %#v, want 16", key, retryBody[key])
		}
	}
}

func TestExecuteNonStreamRetryFailureReturnsOriginalUpstreamResponse(t *testing.T) {
	for _, failure := range []string{"build", "response", "transport"} {
		t.Run(failure, func(t *testing.T) {
			plan, err := protocol.PlanRequest("chat", protocol.OpenAIChat, "auto")
			if err != nil {
				t.Fatal(err)
			}
			prepared := PreparedRequest{Model: "model", ProtocolPlan: plan, Body: map[string]interface{}{"model": "model"}, sourceBody: map[string]interface{}{"model": "model"}}
			calls := 0
			builder := func([]byte) (OutboundRequest, error) {
				calls++
				if calls == 2 && failure == "build" {
					return nil, io.ErrUnexpectedEOF
				}
				status, responseBody, url := http.StatusTooManyRequests, `{"message":"retry failed"}`, "http://upstream.test/retry"
				if calls == 1 {
					status, responseBody, url = http.StatusBadRequest, `{"type":"invalid_request_error","message":"original"}`, "http://upstream.test/original"
				}
				return testOutbound(func(context.Context) (OutboundResponse, error) {
					if calls == 2 && failure == "transport" {
						return OutboundResponse{URL: url}, io.ErrUnexpectedEOF
					}
					response := testResponse(status, responseBody)
					response.URL = url
					return response, nil
				}), nil
			}

			result := ExecuteNonStream(context.Background(), prepared, builder)
			if calls != 2 || result.Kind != NonStreamOutcomeUpstreamResponse || result.Status != http.StatusBadRequest || result.URL != "http://upstream.test/original" || !strings.Contains(string(result.Body), "original") {
				t.Fatalf("outcome = %+v, calls=%d; want original 400 response after retry %s failure", result, calls, failure)
			}
		})
	}
}

func TestExecuteNonStreamBuildFailureIsNotLogged(t *testing.T) {
	plan, err := protocol.PlanRequest("chat", protocol.OpenAIChat, "auto")
	if err != nil {
		t.Fatal(err)
	}
	prepared := PreparedRequest{Model: "model", ProtocolPlan: plan, Body: map[string]interface{}{"model": "model"}}
	result := ExecuteNonStream(context.Background(), prepared, func([]byte) (OutboundRequest, error) {
		return nil, io.ErrUnexpectedEOF
	})
	if result.Status != http.StatusBadGateway || result.Kind != NonStreamOutcomeError || result.ErrorType != "upstream_error" || result.ShouldLog {
		t.Fatalf("outcome = %+v, want unlogged 502 build error", result)
	}
}

func TestExecuteNonStreamCanceledRetryReturnsNoResponse(t *testing.T) {
	plan, err := protocol.PlanRequest("chat", protocol.OpenAIChat, "auto")
	if err != nil {
		t.Fatal(err)
	}
	prepared := PreparedRequest{Model: "model", ProtocolPlan: plan, Body: map[string]interface{}{"model": "model"}, sourceBody: map[string]interface{}{"model": "model"}}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	builder := func([]byte) (OutboundRequest, error) {
		calls++
		if calls == 1 {
			return testOutbound(func(context.Context) (OutboundResponse, error) {
				response := testResponse(http.StatusBadRequest, `{"type":"invalid_request_error"}`)
				response.URL = "http://upstream.test/original"
				return response, nil
			}), nil
		}
		cancel()
		return testOutbound(func(context.Context) (OutboundResponse, error) {
			return OutboundResponse{URL: "http://upstream.test/retry"}, context.Canceled
		}), nil
	}

	result := ExecuteNonStream(ctx, prepared, builder)
	if calls != 2 || result.Kind != NonStreamOutcomeCanceled || result.Status != 499 || !result.ShouldLog || result.URL != "http://upstream.test/retry" {
		t.Fatalf("outcome = %+v, calls=%d; want logged cancellation without response", result, calls)
	}
}
