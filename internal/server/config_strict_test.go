package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

// ARCH-01: a corrupt config must fail the read endpoints explicitly instead of
// being served as a default/empty config.
func TestStrictConfig_CorruptFileFailsReadEndpoints(t *testing.T) {
	cfgPath := isolateConfig(t)
	if err := os.WriteFile(cfgPath, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	r := NewMgmtRouter()
	for _, path := range []string{"/api/config", "/api/state", "/api/profiles", "/api/settings"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("GET %s with corrupt config = %d, want 500 (body=%s)", path, w.Code, w.Body.String())
		}
		// system-contract 2.8: management errors are a bare string message.
		var body struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error == "" {
			t.Fatalf("GET %s management envelope = %s, want {\"error\":\"...\"}", path, w.Body.String())
		}
	}
}

// system-contract 2.8: the inference surface keeps the OpenAI error object even
// when the failure is a shared pre-step such as reading the config.
func TestStrictConfig_InferenceErrorEnvelope(t *testing.T) {
	cfgPath := isolateConfig(t)
	if err := os.WriteFile(cfgPath, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	NewProxyRouter().ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("GET /v1/models with corrupt config = %d, want 500 (body=%s)", w.Code, w.Body.String())
	}
	var body struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode /v1/models error: %v (%s)", err, w.Body.String())
	}
	if body.Error.Message == "" || body.Error.Type == "" {
		t.Fatalf("GET /v1/models inference envelope = %s, want {\"error\":{\"message\":...,\"type\":...}}", w.Body.String())
	}
}

// A missing file is still the default config, so the read endpoints keep working
// before the first save.
func TestStrictConfig_MissingFileStillServesDefault(t *testing.T) {
	isolateConfig(t) // config file intentionally not created
	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/settings without config = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
}

// ARCH-02: PUT /api/config parses the body into the typed config before it
// persists, so a body the strict loader would reject can never be written, and
// the save goes through the 0600 atomic writer.
func TestPutConfig_ParsesBeforePersist(t *testing.T) {
	cfgPath := isolateConfig(t)
	r := NewMgmtRouter()
	put := func(body string) int {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w.Code
	}

	if code := put(`{"version":"two"}`); code != http.StatusBadRequest {
		t.Fatalf("PUT config with a wrong field type = %d, want 400", code)
	}
	if code := put(`{"version":2,"profiles":{"p":{"api":"openai-completions","responsesMode":"passthrough"}}}`); code != http.StatusBadRequest {
		t.Fatalf("PUT config with an invalid responsesMode = %d, want 400", code)
	}
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatalf("a rejected PUT must not create %s (stat err=%v)", cfgPath, err)
	}

	body := `{"version":2,"current":"p","profiles":{"p":{"api":"openai-completions","baseUrl":"https://example.test/v1","apiKey":"k"}},"settings":{"writeMode":"gateway"}}`
	if code := put(body); code != http.StatusOK {
		t.Fatalf("PUT valid config = %d, want 200", code)
	}
	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if mode := info.Mode().Perm(); mode != 0600 {
			t.Fatalf("saved config mode = %o, want 600", mode)
		}
	}
	reloaded, _, err := config.LoadConfigAtPath(cfgPath)
	if err != nil {
		t.Fatalf("strict reload of PUT config: %v", err)
	}
	if _, ok := reloaded.Profiles["p"]; !ok {
		t.Fatalf("saved config lost profile p: %+v", reloaded.Profiles)
	}
	if reloaded.Settings.Proxy.Host != "127.0.0.1" || reloaded.Settings.Proxy.Port != 43112 {
		t.Fatalf("settings defaults not backfilled: %+v", reloaded.Settings.Proxy)
	}
}
