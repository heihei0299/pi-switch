package server

import (
	"encoding/json"
	"fmt"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/store"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func auditFixture(t *testing.T, api, upstreamURL string) config.ProviderProfile {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG", filepath.Join(dir, "config.json"))
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	store.Close()
	t.Cleanup(store.Close)
	name := "main"
	p := config.ProviderProfile{API: api, ResponsesMode: "auto", Upstreams: []config.Upstream{{
		Name: &name, API: api, BaseURL: upstreamURL, ResponsesMode: "auto",
		Models:        []config.ModelEntry{{ID: "audit-model", Cost: &config.ModelCost{Input: 3, Output: 15, CacheRead: 0.3}}},
		ExposedModels: []string{"audit-model"},
	}}}
	cfg := config.DefaultConfig()
	cfg.Profiles = map[string]config.ProviderProfile{"audit": p}
	cfg.Settings.ConversationSource = "off"
	if err := config.SaveAtPath(cfg, config.ResolvePath()); err != nil {
		t.Fatal(err)
	}
	return p
}

func auditRequest(path string, stream bool) *http.Request {
	return httptest.NewRequest(http.MethodPost, path, strings.NewReader(fmt.Sprintf(
		`{"model":"audit-model","messages":[{"role":"user","content":"hello"}],"max_tokens":128,"stream":%t}`, stream)))
}

func TestNonStreamingResponseMatchesClientProtocol(t *testing.T) {
	cases := []struct{ name, api, path, upstream, key string }{
		{"chat_to_responses", "openai-responses", "/v1/chat/completions", `{"id":"resp_a","object":"response","model":"audit-model","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":10,"output_tokens":2}}`, "choices"},
		{"messages_to_chat", "openai-completions", "/v1/messages", `{"id":"chat_a","object":"chat.completion","model":"audit-model","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2}}`, "content"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tc.upstream)
			}))
			defer upstream.Close()
			auditFixture(t, tc.api, upstream.URL)
			w := httptest.NewRecorder()
			NewProxyRouter().ServeHTTP(w, auditRequest(tc.path, false))
			if w.Code != 200 {
				t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
			}
			var got map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(w.Body.String(), "hello") {
				t.Errorf("response text lost: %s", w.Body.String())
			}
			if tc.key == "content" && got["stop_reason"] != "end_turn" {
				t.Errorf("stop_reason = %v, want end_turn", got["stop_reason"])
			}
			if _, ok := got[tc.key]; !ok {
				t.Errorf("client protocol field %q missing; response=%s", tc.key, w.Body.String())
			}
		})
	}
}

func TestResponseConversionFailureReturnsBadGateway(t *testing.T) {
	for _, pair := range []struct{ api, path string }{
		{"openai-responses", "/v1/chat/completions"},
		{"openai-completions", "/v1/messages"},
		{"openai-completions", "/v1/responses"},
		{"anthropic-messages", "/v1/chat/completions"},
	} {
		for _, payload := range []string{`not JSON`, `{"wrong":"shape"}`} {
			for _, retry := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/retry=%t", pair.api, pair.path, retry), func(t *testing.T) {
					calls := 0
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						w.Header().Set("Content-Type", "application/json")
						if retry && calls == 1 {
							w.WriteHeader(http.StatusBadRequest)
							fmt.Fprint(w, `{"error":{"type":"invalid_request_error"}}`)
							return
						}
						fmt.Fprint(w, payload)
					}))
					defer upstream.Close()
					auditFixture(t, pair.api, upstream.URL)
					w := httptest.NewRecorder()
					NewProxyRouter().ServeHTTP(w, auditRequest(pair.path, false))
					if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "conversion_error") {
						t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
					}
					wantCalls := 1
					if retry {
						wantCalls = 2
					}
					if calls != wantCalls {
						t.Fatalf("upstream calls = %d, want %d", calls, wantCalls)
					}
				})
			}
		}
	}
}

func TestChatFinishReasonReachesResponsesClient(t *testing.T) {
	for _, tc := range []struct {
		name, finish, message, itemType, status, reason string
	}{
		{"length text", "length", `{"role":"assistant","content":"partial"}`, "message", "incomplete", "max_output_tokens"},
		{"filtered text", "content_filter", `{"role":"assistant","content":"partial"}`, "message", "incomplete", "content_filter"},
		{"completed text", "stop", `{"role":"assistant","content":"partial"}`, "message", "completed", ""},
		{"length tool", "length", `{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{\"q\":"}}]}`, "function_call", "incomplete", "max_output_tokens"},
		{"completed tool", "tool_calls", `{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{\"q\":\"pi\"}"}}]}`, "function_call", "completed", ""},
	} {
		for _, retry := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/retry=%t", tc.name, retry), func(t *testing.T) {
				calls := 0
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					w.Header().Set("Content-Type", "application/json")
					if retry && calls == 1 {
						w.WriteHeader(http.StatusBadRequest)
						fmt.Fprint(w, `{"error":{"type":"invalid_request_error"}}`)
						return
					}
					fmt.Fprintf(w, `{"model":"audit-model","choices":[{"message":%s,"finish_reason":%q}]}`, tc.message, tc.finish)
				}))
				defer upstream.Close()
				auditFixture(t, "openai-completions", upstream.URL)
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"audit-model","input":"hello","max_output_tokens":128}`))
				NewProxyRouter().ServeHTTP(w, req)
				if w.Code != http.StatusOK {
					t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
				}
				var got map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got["status"] != tc.status {
					t.Errorf("status=%v, want %s", got["status"], tc.status)
				}
				details, _ := got["incomplete_details"].(map[string]interface{})
				if tc.reason != "" && details["reason"] != tc.reason {
					t.Errorf("incomplete_details=%v, want reason %s", details, tc.reason)
				} else if tc.reason == "" && len(details) != 0 {
					t.Errorf("completed response has incomplete_details=%v", details)
				}
				output, _ := got["output"].([]interface{})
				if len(output) != 1 {
					t.Fatalf("output=%v, want one %s item", output, tc.itemType)
				}
				item := output[0].(map[string]interface{})
				if item["type"] != tc.itemType || item["status"] != tc.status {
					t.Errorf("output item=%v, want %s/%s", item, tc.itemType, tc.status)
				}
				if tc.itemType == "message" {
					parts, _ := item["content"].([]interface{})
					if len(parts) != 1 || parts[0].(map[string]interface{})["text"] != "partial" {
						t.Errorf("response text lost: %v", item)
					}
				} else if item["call_id"] != "call-1" || item["name"] != "lookup" || item["arguments"] == nil {
					t.Errorf("tool call lost: %v", item)
				}
			})
		}
	}
}
