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
