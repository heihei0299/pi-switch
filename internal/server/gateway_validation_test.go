package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGatewayPreviewReportsUnsupportedAPI(t *testing.T) {
	dir := t.TempDir()
	cfgJSON := `{"version":2,"profiles":{"sup":{"upstreams":[{"name":"anthropic","api":"anthropic-messages","baseUrl":"https://a.example","models":[{"id":"claude"}],"exposedModels":["claude"]}]}}}`
	writeChannelConfig(t, dir, cfgJSON)
	modelsPath := filepath.Join(dir, "models.json")
	t.Setenv("PI_SWITCH_MODELS", modelsPath)

	w := httptest.NewRecorder()
	NewMgmtRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/models/gateway/preview", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("preview status = %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	conflicts, _ := body["conflicts"].([]interface{})
	if len(conflicts) != 1 || !strings.Contains(conflicts[0].(string), "unsupported API") {
		t.Fatalf("preview conflicts = %#v", conflicts)
	}
}

func TestGatewayPutRejectsDuplicateAndPreservesFile(t *testing.T) {
	dir := t.TempDir()
	cfgJSON := `{"version":2,"profiles":{"alpha":{"upstreams":[{"name":"main","api":"openai-completions","baseUrl":"https://a.example","models":[{"id":"same"}],"exposedModels":["same"]}]},"beta":{"upstreams":[{"name":"backup","api":"openai-completions","baseUrl":"https://b.example","models":[{"id":"same"}],"exposedModels":["same"]}]}}}`
	writeChannelConfig(t, dir, cfgJSON)
	modelsPath := filepath.Join(dir, "models.json")
	original := []byte(`{"providers":{"third-party":{"api":"openai-completions","models":[]}}}`)
	if err := os.WriteFile(modelsPath, original, 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	payload := `{"providers":{"pi-switch-chat":{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","models":[{"id":"same"}]}}}`
	w := httptest.NewRecorder()
	NewMgmtRouter().ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/models/gateway", strings.NewReader(payload)))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "alpha/main") || !strings.Contains(w.Body.String(), "beta/backup") {
		t.Fatalf("duplicate status=%d body=%s", w.Code, w.Body.String())
	}
	got, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("models.json changed on rejected duplicate: %s", got)
	}
}

func TestGatewayPutRejectsThirdProxyProviderAndPreservesFile(t *testing.T) {
	dir := t.TempDir()
	writeChannelConfig(t, dir, `{"version":2,"profiles":{}}`)
	modelsPath := filepath.Join(dir, "models.json")
	original := []byte(`{"providers":{"third-party":{"api":"openai-completions","models":[]}}}`)
	if err := os.WriteFile(modelsPath, original, 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	payload := `{"providers":{"pi-switch-extra":{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","models":[]}}}`
	w := httptest.NewRecorder()
	NewMgmtRouter().ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/models/gateway", strings.NewReader(payload)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	got, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("models.json changed on rejected provider: %s", got)
	}
}

func TestGatewayPutRejectsInvalidModelAndPreservesFile(t *testing.T) {
	dir := t.TempDir()
	writeChannelConfig(t, dir, `{"version":2,"profiles":{}}`)
	modelsPath := filepath.Join(dir, "models.json")
	original := []byte(`{"providers":{"third-party":{"api":"openai-completions","models":[]}}}`)
	if err := os.WriteFile(modelsPath, original, 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	payload := `{"providers":{"pi-switch-chat":{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","models":[1]}}}`
	w := httptest.NewRecorder()
	NewMgmtRouter().ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/models/gateway", strings.NewReader(payload)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	got, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("models.json changed on rejected model: %s", got)
	}
}
