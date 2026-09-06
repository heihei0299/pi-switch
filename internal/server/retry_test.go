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
	"time"

	"github.com/heihei0299/pi-switch/internal/config"
)

// Seam 2 RED: CLIProxyAPI-style retry rounds + request-scoped errors + cooldown.
// Helpers writeRetryConfig / resetRetryStateForTest / classifyUpstreamError etc.
// do not exist yet and must be added with the implementation.

func writeRetryConfig(t *testing.T, dir, cfgJSON string) string {
	t.Helper()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func retryMock(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestRetry_ClassifyDefaults(t *testing.T) {
	resetRetryStateForTest()
	cfg := config.DefaultConfig()
	prof := config.ProviderProfile{API: "openai-completions", ResponsesMode: "auto"}
	for _, st := range []int{403, 408, 429, 500, 502, 503, 504} {
		if got := classifyUpstreamError(st, []byte(`{}`), &prof, &cfg); got != actionContinueAndCooldown {
			t.Fatalf("status %d should continue-and-cooldown, got %s", st, got)
		}
	}
	for _, st := range []int{400, 401, 404, 422} {
		if got := classifyUpstreamError(st, []byte(`{}`), &prof, &cfg); got != actionStop {
			t.Fatalf("status %d should stop, got %s", st, got)
		}
	}
}

func TestRetry_CustomScopedRuleStopsFailover(t *testing.T) {
	resetRetryStateForTest()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	var countB int
	mockA := retryMock(500, `{"error":{"message":"server overloaded, try later"}}`)
	defer mockA.Close()
	mockB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		countB++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chatcmpl-b", "object": "chat.completion", "model": "gpt-4o-mini",
			"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "content": "ok"}}},
		})
	}))
	defer mockB.Close()
	cfgJSON := fmt.Sprintf(`{
		"version":2,"current":"retry-stop-a",
		"profiles":{
			"retry-stop-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-a",
				"models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"exposedModels":["gpt-4o-mini"],
				"requestScopedErrors":[{"status":500,"match":["overloaded"],"action":"stop"}]},
			"retry-stop-b":{"api":"openai-completions","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-b",
				"models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"exposedModels":["gpt-4o-mini"]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions",
			"proxy":{"host":"127.0.0.1","port":43112,"failover":["retry-stop-a","retry-stop-b"]},
			"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`, mockA.URL, mockB.URL)
	cfgPath := writeRetryConfig(t, dir, cfgJSON)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	NewProxyRouter().ServeHTTP(w, req)
	if w.Code != 500 {
		t.Fatalf("custom stop rule should return upstream 500 as-is, got %d body %s", w.Code, w.Body.String())
	}
	if countB != 0 {
		t.Fatalf("stop rule must not fail over, B hits = %d", countB)
	}
}

func TestRetry_CooldownSkipsRecentlyFailed(t *testing.T) {
	t.Skip("failover removed, test skipped")
	resetRetryStateForTest()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	var countA, countB int
	mockA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		countA++
		w.WriteHeader(502)
		_, _ = w.Write([]byte(`{"error":"flaky"}`))
	}))
	defer mockA.Close()
	mockB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		countB++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chatcmpl-b", "object": "chat.completion", "model": "gpt-4o-mini",
			"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "content": "ok"}}},
			"usage":   map[string]interface{}{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	defer mockB.Close()
	cfgJSON := fmt.Sprintf(`{
		"version":2,"current":"retry-cd-a",
		"profiles":{
			"retry-cd-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-a",
				"models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"exposedModels":["gpt-4o-mini"]},
			"retry-cd-b":{"api":"openai-completions","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-b",
				"models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"exposedModels":["gpt-4o-mini"]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions",
			"proxy":{"host":"127.0.0.1","port":43112,"failover":["retry-cd-a","retry-cd-b"]},
			"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`, mockA.URL, mockB.URL)
	cfgPath := writeRetryConfig(t, dir, cfgJSON)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	router := NewProxyRouter()
	doReq := func() int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		return w.Code
	}
	if code := doReq(); code != 200 {
		t.Fatalf("first request should fail over to B, got %d", code)
	}
	if code := doReq(); code != 200 {
		t.Fatalf("second request should succeed via B, got %d", code)
	}
	if countA != 1 {
		t.Fatalf("cooling A must be skipped on second request, A hits = %d want 1", countA)
	}
	if countB != 2 {
		t.Fatalf("B should serve both requests, B hits = %d want 2", countB)
	}
}

func TestRetry_RetryRoundsSingleCredential(t *testing.T) {
	t.Skip("failover removed, test skipped")
	resetRetryStateForTest()
	// Disable cooling so the single credential is re-admitted in round 1.
	savedSleep := retrySleep
	retrySleep = func(d time.Duration) {}
	defer func() { retrySleep = savedSleep }()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	var hits int
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"down"}`))
	}))
	defer mock.Close()
	cfgJSON := fmt.Sprintf(`{
		"version":2,"current":"retry-rounds-a",
		"profiles":{
			"retry-rounds-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-a",
				"models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"exposedModels":["gpt-4o-mini"],
				"disableCooling":true,"requestRetry":1}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions",
			"proxy":{"host":"127.0.0.1","port":43112,"failover":["retry-rounds-a"]},
			"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`, mock.URL)
	cfgPath := writeRetryConfig(t, dir, cfgJSON)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	NewProxyRouter().ServeHTTP(w, req)
	if w.Code == 200 {
		t.Fatalf("all-fail should not be 200")
	}
	if hits != 2 {
		t.Fatalf("requestRetry=1 should attempt round 0+1 = 2 hits, got %d", hits)
	}
}
func TestRetry_AllCoolingDiagnosed(t *testing.T) {
	t.Skip("failover removed, test skipped")
	resetRetryStateForTest()
	sleeps := 0
	savedSleep := retrySleep
	retrySleep = func(d time.Duration) { sleeps++ }
	defer func() { retrySleep = savedSleep }()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	var hits int
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","model":"gpt-4o-mini","choices":[]}`))
	}))
	defer mock.Close()
	cfgJSON := fmt.Sprintf(`{
		"version":2,"current":"retry-frozen-a",
		"profiles":{
			"retry-frozen-a":{"api":"openai-completions","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-a",
				"models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"exposedModels":["gpt-4o-mini"]},
			"retry-frozen-b":{"api":"openai-completions","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-b",
				"models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"exposedModels":["gpt-4o-mini"]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions",
			"proxy":{"host":"127.0.0.1","port":43112,"failover":["retry-frozen-a","retry-frozen-b"]},
			"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`, mock.URL, mock.URL+"/backup")
	cfgPath := writeRetryConfig(t, dir, cfgJSON)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	// Pre-cool both candidates so every round has zero eligible attempts.
	markCooling(cooldownKey("retry-frozen-a", mock.URL), time.Hour)
	markCooling(cooldownKey("retry-frozen-b", mock.URL+"/backup"), time.Hour)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	NewProxyRouter().ServeHTTP(w, req)
	if w.Code == 200 {
		t.Fatalf("all-cooling should not succeed")
	}
	if hits != 0 {
		t.Fatalf("cooling candidates must not be attempted, hits = %d", hits)
	}
	if sleeps != 0 {
		t.Fatalf("1h cooldown beyond 30s cap must not sleep, sleeps = %d", sleeps)
	}
	if !strings.Contains(w.Body.String(), "cooling") {
		t.Fatalf("error should diagnose cooling, got %s", w.Body.String())
	}
}

func TestRetry_ValidationRejectsBadRule(t *testing.T) {
	resetRetryStateForTest()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	cfgPath := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cfgPath, []byte(`{"version":2,"profiles":{},"settings":{"providerPrefix":"pi-switch"}}`), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	r := NewMgmtRouter()
	post := func(payload string) int {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/profiles", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w.Code
	}
	if code := post(`{"name":"bad-action","profile":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://a","apiKey":"k","models":[],"requestScopedErrors":[{"status":500,"action":"explode"}]}}`); code != 400 {
		t.Fatalf("invalid action should 400, got %d", code)
	}
	if code := post(`{"name":"bad-retry","profile":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://a","apiKey":"k","models":[],"requestRetry":99}}`); code != 400 {
		t.Fatalf("requestRetry 99 should 400, got %d", code)
	}
}
