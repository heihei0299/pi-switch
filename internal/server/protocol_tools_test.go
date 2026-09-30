package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponsesFunctionStrictReachesChatUpstream(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		value       interface{}
		present     bool
	}{
		{"enabled", `,"strict":true`, true, true},
		{"disabled", `,"strict":false`, false, true},
		{"omitted", "", nil, false},
		{"null", `,"strict":null`, nil, true},
	} {
		for _, retry := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/retry=%t", tc.name, retry), func(t *testing.T) {
				requests := make(chan map[string]interface{}, 2)
				calls := 0
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("decode upstream request: %v", err)
						http.Error(w, "invalid JSON", http.StatusBadRequest)
						return
					}
					requests <- body
					calls++
					w.Header().Set("Content-Type", "application/json")
					if retry && calls == 1 {
						w.WriteHeader(http.StatusBadRequest)
						fmt.Fprint(w, `{"error":{"type":"invalid_request_error"}}`)
						return
					}
					fmt.Fprint(w, `{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}]}`)
				}))
				defer upstream.Close()
				auditFixture(t, "openai-completions", upstream.URL)
				w := httptest.NewRecorder()
				body := fmt.Sprintf(`{"model":"audit-model","input":"hello","max_output_tokens":128,"tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{"q":{"type":"string"}},"required":["q"],"additionalProperties":false}%s}]}`, tc.field)
				NewProxyRouter().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body)))
				if w.Code != http.StatusOK {
					t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
				}
				wantCalls := 1
				if retry {
					wantCalls = 2
				}
				if len(requests) != wantCalls {
					t.Fatalf("upstream requests=%d, want %d", len(requests), wantCalls)
				}
				for i := 0; i < wantCalls; i++ {
					request := <-requests
					tools, _ := request["tools"].([]interface{})
					if len(tools) != 1 {
						t.Fatalf("upstream tools=%v, want one function", tools)
					}
					function, _ := tools[0].(map[string]interface{})["function"].(map[string]interface{})
					strict, present := function["strict"]
					if present != tc.present || strict != tc.value {
						t.Errorf("attempt %d strict=%v/present=%t, want %v/%t", i+1, strict, present, tc.value, tc.present)
					}
				}
			})
		}
	}
}
