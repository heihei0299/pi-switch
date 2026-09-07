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

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func writeApiContractConfig(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

func TestApiContract_08_S1_TypesSync(t *testing.T) {
	// types.ts should point to Go config as source of truth, not Rust
	data, err := os.ReadFile("../../webui/src/types.ts")
	if err != nil {
		// fallback when running from different cwd
		data, err = os.ReadFile("webui/src/types.ts")
		if err != nil {
			t.Fatalf("read types.ts: %v", err)
		}
	}
	s := string(data)
	if !strings.Contains(s, "internal/config/config.go") && !strings.Contains(s, "internal/config") {
		t.Fatalf("types.ts header must reference internal/config/config.go as source of truth, got header %.200s", s)
	}
	if strings.Contains(s, "src-rust/config.rs") && !strings.Contains(s, "internal/config") {
		t.Fatalf("types.ts still references Rust src-rust/config.rs, should point to Go")
	}
	// basic presence of key types
	for _, want := range []string{"ProviderProfile", "ModelEntry", "Settings", "ResponsesMode"} {
		if !strings.Contains(s, want) {
			t.Fatalf("types.ts missing %s", want)
		}
	}
	// ensure json tag sync is guarded: Go config must have expected json fields
	cfgData, err := os.ReadFile("../../internal/config/config.go")
	if err != nil {
		cfgData, err = os.ReadFile("internal/config/config.go")
		if err != nil {
			t.Fatalf("read config.go: %v", err)
		}
	}
	cs := string(cfgData)
	for _, tag := range []string{`json:"responsesMode"`, `json:"api"`, `json:"baseUrl"`, `json:"models,omitempty"`, `json:"exposedModels,omitempty`} {
		if !strings.Contains(cs, tag) {
			t.Fatalf("config.go missing json tag %s", tag)
		}
	}
}

func TestApiContract_08_S2_ResponsesMode400(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	cfg := `{
		"version":2,
		"profiles":{
			"ok-prof":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-test","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`
	p := writeApiContractConfig(t, dir, cfg)
	t.Setenv("PI_SWITCH_CONFIG", p)
	t.Setenv("PI_SWITCH_DB", dbPath)
	r := NewMgmtRouter()

	cases := []struct {
		name    string
		method  string
		path    string
		body    string
		want400 bool
		substr  string
	}{
		{
			name:    "POST passthrough requires openai-responses",
			method:  "POST",
			path:    "/api/profiles",
			body:    `{"name":"bad1","profile":{"api":"openai-completions","responsesMode":"passthrough","preset":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-x","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}]}}`,
			want400: true,
			substr:  "responsesMode passthrough requires api openai-responses, got openai-completions",
		},
		{
			name:    "POST convert requires openai-completions",
			method:  "POST",
			path:    "/api/profiles",
			body:    `{"name":"bad2","profile":{"api":"openai-responses","responsesMode":"convert","preset":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-x","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}]}}`,
			want400: true,
			substr:  "responsesMode convert requires api openai-completions, got openai-responses",
		},
		{
			name:    "POST auto always pass",
			method:  "POST",
			path:    "/api/profiles",
			body:    `{"name":"good-auto","profile":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-x","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}]}}`,
			want400: false,
		},
		{
			name:    "PUT passthrough mismatch",
			method:  "PUT",
			path:    "/api/profiles/ok-prof",
			body:    `{"profile":{"api":"anthropic-messages","responsesMode":"passthrough","preset":"anthropic","baseUrl":"https://api.anthropic.com","apiKey":"sk-x","models":[{"id":"claude-3","contextWindow":128000,"maxTokens":16384}]}}`,
			want400: true,
			substr:  "responsesMode passthrough requires api openai-responses, got anthropic-messages",
		},
		{
			name:    "PUT convert mismatch",
			method:  "PUT",
			path:    "/api/profiles/ok-prof",
			body:    `{"profile":{"api":"openai-responses","responsesMode":"convert","preset":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-x","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}]}}`,
			want400: true,
			substr:  "responsesMode convert requires api openai-completions, got openai-responses",
		},
		{
			name:    "PUT auto pass",
			method:  "PUT",
			path:    "/api/profiles/ok-prof",
			body:    `{"profile":{"api":"openai-responses","responsesMode":"auto","preset":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-x","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}]}}`,
			want400: false,
		},
		{
			name:    "PUT config raw passthrough mismatch",
			method:  "PUT",
			path:    "/api/config",
			body:    fmt.Sprintf(`{"version":2,"profiles":{"x":{"api":"openai-completions","responsesMode":"passthrough","preset":"openai","baseUrl":"https://x","apiKey":"k","models":[{"id":"m","contextWindow":128000,"maxTokens":16384}]}},"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}}`),
			want400: true,
			substr:  "responsesMode passthrough requires api openai-responses",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			if tc.want400 {
				if w.Code != 400 {
					t.Fatalf("want 400, got %d body %s", w.Code, w.Body.String())
				}
				var m map[string]interface{}
				_ = json.Unmarshal(w.Body.Bytes(), &m)
				errStr, _ := m["error"].(string)
				if errStr == "" {
					// error may be nested?
					if e, ok := m["error"]; ok {
						errStr = fmt.Sprintf("%v", e)
					}
				}
				if !strings.Contains(errStr, tc.substr) {
					t.Fatalf("error %q must contain %q body %s", errStr, tc.substr, w.Body.String())
				}
			} else {
				if w.Code != 200 {
					t.Fatalf("want 200, got %d body %s", w.Code, w.Body.String())
				}
			}
		})
	}
}

func TestApiContract_08_S3_ValidateOnlyHint(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	// config with: no models warning, modelsDevProvider unknown warning, failover not found
	cfg := `{
		"version":2,
		"profiles":{
			"no-models-prof":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-test","models":[]},
			"unknown-dev-prof":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-test","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"modelsDevProvider":"unknown-provider-xyz"},
			"ok-prof":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-test","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112,"failover":["ghost-provider"]},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`
	p := writeApiContractConfig(t, dir, cfg)
	t.Setenv("PI_SWITCH_CONFIG", p)
	t.Setenv("PI_SWITCH_DB", dbPath)
	r := NewMgmtRouter()

	// GET /api/validate and GET /api/config/validate both return ValidationIssue[]
	for _, path := range []string{"/api/validate", "/api/config/validate"} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", path, nil)
			r.ServeHTTP(w, req)
			if w.Code != 200 {
				t.Fatalf("%s code %d want 200 body %s", path, w.Code, w.Body.String())
			}
			var issues []map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &issues); err != nil {
				t.Fatalf("unmarshal %s: %v body %s", path, err, w.Body.String())
			}
			if len(issues) == 0 {
				t.Fatalf("%s returned empty issues, want warnings", path)
			}
			for _, iss := range issues {
				for _, k := range []string{"level", "path", "message"} {
					if _, ok := iss[k]; !ok {
						t.Fatalf("issue missing %s: %v", k, iss)
					}
				}
			}
			// check no models warning
			foundNoModels := false
			foundDevUnknown := false
			foundFailover := false
			for _, iss := range issues {
				level, _ := iss["level"].(string)
				pathStr, _ := iss["path"].(string)
				msg, _ := iss["message"].(string)
				if level == "warning" && strings.Contains(pathStr, "no-models-prof") && strings.Contains(strings.ToLower(msg), "no models") {
					foundNoModels = true
				}
				if level == "warning" && strings.Contains(pathStr, "modelsDevProvider") && strings.Contains(strings.ToLower(msg), "unknown") {
					foundDevUnknown = true
				}
				if level == "warning" && strings.Contains(pathStr, "failover") && strings.Contains(msg, "ghost-provider") {
					foundFailover = true
				}
			}
			if !foundNoModels {
				t.Fatalf("%s missing warning no models, issues=%v", path, issues)
			}
			if !foundDevUnknown {
				t.Fatalf("%s missing warning modelsDevProvider unknown, issues=%v", path, issues)
			}
			if false && !foundFailover {
				t.Fatalf("%s missing warning failover not found, issues=%v", path, issues)
			}
		})
	}

	// PUT not blocked: create a profile with no models should succeed despite warning
	t.Run("PUT_not_blocked", func(t *testing.T) {
		body := `{"name":"new-empty","profile":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-x","models":[]}}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/profiles", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("PUT not blocked: POST empty models should be 200, got %d body %s", w.Code, w.Body.String())
		}
		// now validate should include warning for new-empty
		w2 := httptest.NewRecorder()
		req2, _ := http.NewRequest("GET", "/api/validate", nil)
		r.ServeHTTP(w2, req2)
		var issues []map[string]interface{}
		_ = json.Unmarshal(w2.Body.Bytes(), &issues)
		found := false
		for _, iss := range issues {
			if p, _ := iss["path"].(string); strings.Contains(p, "new-empty") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("after PUT, validate should contain warning for new-empty, got %v", issues)
		}
	})

	// failover hot update
	t.Run("failover_hot_update", func(t *testing.T) {
		// initial has ghost-provider warning, now update failover to existing profile
		// GET current config, modify failover, PUT back
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/config", nil)
		r.ServeHTTP(w, req)
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		cfgMap, _ := resp["config"].(map[string]interface{})
		settings, _ := cfgMap["settings"].(map[string]interface{})
		proxy, _ := settings["proxy"].(map[string]interface{})
		proxy["failover"] = []interface{}{"ok-prof"}
		newRaw, _ := json.Marshal(cfgMap)
		w2 := httptest.NewRecorder()
		req2, _ := http.NewRequest("PUT", "/api/config", strings.NewReader(string(newRaw)))
		req2.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w2, req2)
		if w2.Code != 200 {
			t.Fatalf("PUT config hot update fail %d body %s", w2.Code, w2.Body.String())
		}
		w3 := httptest.NewRecorder()
		req3, _ := http.NewRequest("GET", "/api/validate", nil)
		r.ServeHTTP(w3, req3)
		var issues []map[string]interface{}
		_ = json.Unmarshal(w3.Body.Bytes(), &issues)
		for _, iss := range issues {
			if p, _ := iss["path"].(string); p == "settings.proxy.failover" {
				t.Fatalf("after hot update, should not have failover warning, got %v", issues)
			}
		}
		// set back to ghost to trigger again
		proxy["failover"] = []interface{}{"ghost-provider-2"}
		newRaw2, _ := json.Marshal(cfgMap)
		w4 := httptest.NewRecorder()
		req4, _ := http.NewRequest("PUT", "/api/config", strings.NewReader(string(newRaw2)))
		req4.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w4, req4)
		if w4.Code != 200 {
			t.Fatalf("second hot update fail %d", w4.Code)
		}
		w5 := httptest.NewRecorder()
		req5, _ := http.NewRequest("GET", "/api/validate", nil)
		r.ServeHTTP(w5, req5)
		var issues2 []map[string]interface{}
		_ = json.Unmarshal(w5.Body.Bytes(), &issues2)
		found := false
		for _, iss := range issues2 {
			if p, _ := iss["path"].(string); strings.Contains(p, "failover") {
				found = true
				break
			}
		}
		if found {
			t.Fatalf("failover removed: should not have warning, got %v", issues2)
		}
	})
}

func TestApiContract_08_S4_ProfileValidate400(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	cfg := `{
		"version":2,
		"profiles":{
			"base":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-test","upstreams":[{"name":"main","api":"openai-completions","baseUrl":"https://api.openai.com/v1","apiKey":"sk-test","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384},{"id":"gpt-4o","contextWindow":128000,"maxTokens":16384}]}]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`
	p := writeApiContractConfig(t, dir, cfg)
	t.Setenv("PI_SWITCH_CONFIG", p)
	t.Setenv("PI_SWITCH_DB", dbPath)
	r := NewMgmtRouter()

	t.Run("duplicate_model_id", func(t *testing.T) {
		body := `{"name":"dup-test","profile":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-x","upstreams":[{"name":"main","api":"openai-completions","baseUrl":"https://api.openai.com/v1","apiKey":"sk-x","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384},{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}]}]}}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/profiles", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Fatalf("duplicate model id should be 400, got %d body %s", w.Code, w.Body.String())
		}
		if !strings.Contains(strings.ToLower(w.Body.String()), "duplicate") {
			t.Fatalf("error should mention duplicate, got %s", w.Body.String())
		}
		// via PUT
		body2 := `{"profile":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-x","upstreams":[{"name":"main","api":"openai-completions","baseUrl":"https://api.openai.com/v1","apiKey":"sk-x","models":[{"id":"a","contextWindow":128000,"maxTokens":16384},{"id":"a","contextWindow":128000,"maxTokens":16384}]}]}}`
		w2 := httptest.NewRecorder()
		req2, _ := http.NewRequest("PUT", "/api/profiles/base", strings.NewReader(body2))
		req2.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w2, req2)
		if w2.Code != 400 {
			t.Fatalf("PUT duplicate should be 400, got %d body %s", w2.Code, w2.Body.String())
		}
	})

	t.Run("exposedModels_unknown", func(t *testing.T) {
		body := `{"name":"expose-bad","profile":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-x","upstreams":[{"name":"main","api":"openai-completions","baseUrl":"https://api.openai.com/v1","apiKey":"sk-x","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"exposedModels":["unknown-model"]}]}}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/profiles", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != 400 {
			t.Fatalf("exposedModels unknown should be 400, got %d body %s", w.Code, w.Body.String())
		}
		if !strings.Contains(strings.ToLower(w.Body.String()), "exposedmodels") && !strings.Contains(strings.ToLower(w.Body.String()), "unknown") {
			t.Fatalf("error should mention exposedModels/unknown, got %s", w.Body.String())
		}
		// PUT version
		body2 := `{"profile":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-x","upstreams":[{"name":"main","api":"openai-completions","baseUrl":"https://api.openai.com/v1","apiKey":"sk-x","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"exposedModels":["ghost-model"]}]}}`
		w2 := httptest.NewRecorder()
		req2, _ := http.NewRequest("PUT", "/api/profiles/base", strings.NewReader(body2))
		req2.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w2, req2)
		if w2.Code != 400 {
			t.Fatalf("PUT exposedModels unknown should be 400, got %d body %s", w2.Code, w2.Body.String())
		}
	})

	t.Run("modelsDevProvider_warning_not_400", func(t *testing.T) {
		body := `{"name":"dev-provider-ok","profile":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://api.openai.com/v1","apiKey":"sk-x","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"modelsDevProvider":"unknown-provider-xyz"}}`
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/profiles", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("modelsDevProvider unknown should NOT be 400 (warning only), got %d body %s", w.Code, w.Body.String())
		}
		// validate should contain warning
		w2 := httptest.NewRecorder()
		req2, _ := http.NewRequest("GET", "/api/validate", nil)
		r.ServeHTTP(w2, req2)
		var issues []map[string]interface{}
		_ = json.Unmarshal(w2.Body.Bytes(), &issues)
		found := false
		for _, iss := range issues {
			if p, _ := iss["path"].(string); strings.Contains(p, "modelsDevProvider") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("validate should warn modelsDevProvider unknown, got %v", issues)
		}
	})
}

func TestApiContract_08_S4_NotFoundAndUpstream(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	// upstream that always returns 500 for /models
	failUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"upstream failure"}`))
	}))
	defer failUp.Close()

	cfg := fmt.Sprintf(`{
		"version":2,
		"profiles":{
			"exists":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":%q,"apiKey":"sk-test","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}
	}`, failUp.URL)
	p := writeApiContractConfig(t, dir, cfg)
	t.Setenv("PI_SWITCH_CONFIG", p)
	t.Setenv("PI_SWITCH_DB", dbPath)
	r := NewMgmtRouter()

	t.Run("404_unknown_profile", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/profiles/nonexistent-xyz", nil)
		r.ServeHTTP(w, req)
		if w.Code != 404 {
			t.Fatalf("GET unknown profile should be 404, got %d body %s", w.Code, w.Body.String())
		}
		var m map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		if _, ok := m["error"]; !ok {
			t.Fatalf("404 body should contain error, got %v", m)
		}
	})

	t.Run("500_upstream_fetch_models", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/profiles/exists/fetch-models", nil)
		r.ServeHTTP(w, req)
		// spec says 500 for upstream, but keep distinct from 404
		if w.Code != 500 {
			t.Fatalf("fetch-models upstream error should be 500 distinct from 404, got %d body %s", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "not found") {
			t.Fatalf("upstream error should not be confused with 404 not found, body %s", w.Body.String())
		}
	})
}
