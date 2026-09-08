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

func TestGatewayPreviewSelectionUsesSourceChannel(t *testing.T) {
	dir := t.TempDir()
	cfgJSON := `{"version":2,"profiles":{"sup":{"api":"openai-completions","upstreams":[
		{"name":"main","api":"openai-completions","baseUrl":"https://main.example/v1","models":[{"id":"m1","contextWindow":100,"maxTokens":10}],"exposedModels":["m1"]},
		{"name":"backup","api":"openai-completions","baseUrl":"https://backup.example/v1","models":[{"id":"m2","contextWindow":200,"maxTokens":20}],"exposedModels":["m2"]}
	]}},"settings":{"proxy":{"host":"127.0.0.1","port":43112}}}`
	writeChannelConfig(t, dir, cfgJSON)
	modelsPath := filepath.Join(dir, "models.json")
	catalogPath := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(catalogPath, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	t.Setenv("PI_SWITCH_CATALOG", catalogPath)

	body := `{"selected":[{"supplier":"sup","channel":"backup","model":"m2"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/models/gateway/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	NewMgmtRouter().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("preview status = %d body=%s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	proposed, ok := response["proposed"].(map[string]interface{})
	if !ok {
		t.Fatalf("proposed = %#v", response["proposed"])
	}
	chat, ok := proposed["pi-switch-chat"].(map[string]interface{})
	if !ok {
		t.Fatalf("chat provider = %#v", proposed["pi-switch-chat"])
	}
	models, ok := chat["models"].([]interface{})
	if !ok || len(models) != 1 {
		t.Fatalf("selected models = %#v", chat["models"])
	}
	if got := models[0].(map[string]interface{})["id"]; got != "m2" {
		t.Fatalf("selected model id = %v, want m2", got)
	}
}

func TestGatewayPreviewSelectionPreservesEditedGatewayExtras(t *testing.T) {
	dir := t.TempDir()
	cfgJSON := `{"version":2,"profiles":{"sup":{"api":"openai-completions","upstreams":[
		{"name":"main","api":"openai-completions","baseUrl":"https://main.example/v1","models":[{"id":"m1","contextWindow":100,"maxTokens":10}],"exposedModels":["m1"]}
	]}},"settings":{"proxy":{"host":"127.0.0.1","port":43112}}}`
	writeChannelConfig(t, dir, cfgJSON)
	modelsPath := filepath.Join(dir, "models.json")
	catalogPath := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(catalogPath, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	t.Setenv("PI_SWITCH_CATALOG", catalogPath)

	body := `{"selected":[{"supplier":"sup","channel":"main","model":"m1"}],"draft":{"providers":{"pi-switch-chat":{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","compat":{"sendSessionAffinityHeaders":true},"models":[{"id":"m1","manual":"keep"}],"proxy":false}}}}`
	req := httptest.NewRequest(http.MethodPost, "/api/models/gateway/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	NewMgmtRouter().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("preview status = %d body=%s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	proposed := response["proposed"].(map[string]interface{})
	chat := proposed["pi-switch-chat"].(map[string]interface{})
	if got := chat["compat"].(map[string]interface{})["sendSessionAffinityHeaders"]; got != true {
		t.Fatalf("compat = %v, want preserved true", got)
	}
	model := chat["models"].([]interface{})[0].(map[string]interface{})
	if model["manual"] != "keep" {
		t.Fatalf("model extras = %#v, want manual=keep", model)
	}
}
