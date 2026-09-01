package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestHealthz(t *testing.T) {
	r := NewProxyRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/healthz", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("proxy /healthz code = %d, want 200", w.Code)
	}
	var m map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	if m["status"] != "ok" {
		t.Fatalf("status = %v, want ok", m["status"])
	}
	r2 := NewMgmtRouter()
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/healthz", nil)
	r2.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("mgmt /healthz code = %d, want 200", w2.Code)
	}
}

func TestRootAndApiConfig(t *testing.T) {
	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("GET / code = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("PI_SWITCH_CONFIG", path)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/config", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("GET /api/config code = %d, want 200", w2.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w2.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := resp["source"]; !ok {
		t.Fatalf("missing source")
	}
	cfg, ok := resp["config"].(map[string]interface{})
	if !ok {
		t.Fatalf("config not map")
	}
	settings, ok := cfg["settings"].(map[string]interface{})
	if !ok {
		t.Fatalf("settings not map")
	}
	if settings["providerPrefix"] != "pi-switch" {
		t.Fatalf("providerPrefix = %v, want pi-switch", settings["providerPrefix"])
	}
	_ = os.WriteFile(path, []byte(`{"version":2,"profiles":{},"settings":{"providerPrefix":"hot-reload-test"}}`), 0644)
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/api/config", nil)
	r.ServeHTTP(w3, req3)
	var resp3 map[string]interface{}
	_ = json.Unmarshal(w3.Body.Bytes(), &resp3)
	cfg3 := resp3["config"].(map[string]interface{})
	settings3 := cfg3["settings"].(map[string]interface{})
	if settings3["providerPrefix"] != "hot-reload-test" {
		t.Fatalf("hot reload failed: %v", settings3["providerPrefix"])
	}
}
