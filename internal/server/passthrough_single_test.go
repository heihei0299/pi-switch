package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPassthrough_429Isolated verifies that 429 is returned directly without
// failing over to another supplier and without polluting the next request.
func TestPassthrough_429Isolated(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	t.Setenv("PI_SWITCH_CONFIG", filepath.Join(dir, "config.json"))

	// Mock A always returns 429, Mock B would return 200 if hit (should not be hit)
	var countA, countB int
	mockA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		countA++
		w.WriteHeader(429)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": map[string]interface{}{"message": "rate limited", "type": "rate_limit"}})
	}))
	defer mockA.Close()
	mockB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		countB++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chatcmpl-b", "object": "chat.completion", "model": "gpt-4o-mini",
			"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "content": "b"}}},
			"usage":   map[string]interface{}{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	defer mockB.Close()

	current := "supplier-a"
	cfg := fmt.Sprintf(`{
		"version":2,
		"current":%q,
		"profiles":{
			"supplier-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-a","upstreams":[{"name":"main","api":"openai-completions","baseUrl":%q,"apiKey":"sk-a","models":[{"id":"gpt-4o-mini"}],"exposedModels":["gpt-4o-mini"]}]},
			"supplier-b":{"api":"openai-completions","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-b","upstreams":[{"name":"main","api":"openai-completions","baseUrl":%q,"apiKey":"sk-b","models":[{"id":"other-model"}],"exposedModels":["other-model"]}]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","conversationSource":"off","proxy":{"host":"127.0.0.1","port":43112,"circuitBreaker":{"enabled":true,"failureThreshold":3,"cooldownSeconds":60}},"web":{"host":"127.0.0.1","port":43110}}
	}`, current, mockA.URL+"/v1", mockA.URL+"/v1", mockB.URL+"/v1", mockB.URL+"/v1")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	// Ensure DB path via env or default? Use temp dir for requests.db via override of internal storage?
	// The server uses default DB path (~/.pi-switch/requests.db), but we can still test response without DB check.
	_ = dbPath
	// reset retry state
	resetRetryStateForTest()

	router := NewProxyRouter()
	// First request with Bare model gpt-4o-mini -> should route to supplier-a (current) and get 429 directly
	body := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != 429 {
		t.Fatalf("first request: got %d body %s, want 429; countA=%d countB=%d", w.Code, w.Body.String(), countA, countB)
	}
	if countB != 0 {
		t.Fatalf("first request should not hit supplier-b, countB=%d", countB)
	}
	if countA != 1 {
		t.Fatalf("first request should hit supplier-a once, countA=%d", countA)
	}
	// Second request immediately after 429 should still hit supplier-a and get 429 (not failover, not cooling-blocked)
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w2, req2)
	if w2.Code != 429 {
		t.Fatalf("second request: got %d body %s, want 429", w2.Code, w2.Body.String())
	}
	if countA != 2 {
		t.Fatalf("second request should hit supplier-a again, countA=%d want 2", countA)
	}
	if countB != 0 {
		t.Fatalf("second request should still not hit supplier-b, countB=%d", countB)
	}
}
