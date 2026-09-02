package server

import (
	"bytes"
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

func TestSettings_GetAndPut_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("PI_SWITCH_CONFIG", path)
	os.Remove(path)
	r := NewMgmtRouter()

	// PUT with proxy
	putPayload := map[string]interface{}{
		"providerPrefix":     "pi-switch",
		"writeMode":          "gateway",
		"gatewayApi":         "openai-completions",
		"proxy":              map[string]interface{}{"host": "127.0.0.1", "port": 43112},
		"web":                map[string]interface{}{"host": "127.0.0.1", "port": 43110},
		"conversationSource": "proxy",
	}
	b, _ := json.Marshal(putPayload)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("PUT", "/api/settings", bytes.NewReader(b))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("PUT proxy status %d body %s", w2.Code, w2.Body.String())
	}
	// GET should return proxy
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/api/settings", nil)
	r.ServeHTTP(w3, req3)
	if w3.Code != 200 {
		t.Fatalf("GET status %d body %s", w3.Code, w3.Body.String())
	}
	var got map[string]interface{}
	_ = json.Unmarshal(w3.Body.Bytes(), &got)
	if got["conversationSource"] != "proxy" {
		t.Fatalf("GET conversationSource = %v, want proxy", got["conversationSource"])
	}

	// PUT without conversationSource should default to sessionScan
	putPayload2 := map[string]interface{}{
		"providerPrefix": "pi-switch",
		"writeMode":      "gateway",
		"gatewayApi":     "openai-completions",
		"proxy":          map[string]interface{}{"host": "127.0.0.1", "port": 43112},
		"web":            map[string]interface{}{"host": "127.0.0.1", "port": 43110},
	}
	b2, _ := json.Marshal(putPayload2)
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest("PUT", "/api/settings", bytes.NewReader(b2))
	req4.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w4, req4)
	if w4.Code != 200 {
		t.Fatalf("PUT missing status %d", w4.Code)
	}
	w5 := httptest.NewRecorder()
	req5, _ := http.NewRequest("GET", "/api/settings", nil)
	r.ServeHTTP(w5, req5)
	var got2 map[string]interface{}
	_ = json.Unmarshal(w5.Body.Bytes(), &got2)
	if got2["conversationSource"] != "sessionScan" {
		t.Fatalf("GET after missing => %v, want sessionScan", got2["conversationSource"])
	}
}
