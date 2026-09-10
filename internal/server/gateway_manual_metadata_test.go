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

func TestGatewayManualMetadataConvergesAfterReload(t *testing.T) {
	dir := t.TempDir()
	cfg := `{"version":2,"profiles":{"sup":{"api":"openai-completions","upstreams":[{"name":"main","api":"openai-completions","baseUrl":"https://upstream.example/v1","apiKey":"key","models":[{"id":"model-a","name":"Model A","contextWindow":1000,"maxTokens":100},{"id":"model-b","name":"Model B","contextWindow":1000,"maxTokens":100}],"exposedModels":["model-a","model-b"]}]}},"settings":{"proxy":{"host":"127.0.0.1","port":43112}}}`
	writeChannelConfig(t, dir, cfg)
	modelsPath := filepath.Join(dir, "models.json")
	catalogPath := filepath.Join(dir, "catalog.json")
	if err := os.WriteFile(catalogPath, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	t.Setenv("PI_SWITCH_CATALOG", catalogPath)

	payload := `{"providers":{"pi-switch-chat":{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","proxy":false,"models":[{"id":"model-a","name":"Model A","contextWindow":1000,"maxTokens":100},{"id":"model-b","name":"Model B Browser Edited","contextWindow":2000,"maxTokens":200,"reasoning":true,"input":["text","image"],"thinkingLevelMap":{"high":"medium"},"cost":{"input":0.5,"output":1.5,"cacheRead":0.2,"cacheWrite":0.1},"compat":{"manual":true},"headers":{"X-Model":"browser"},"extra":{"e2e":"browser"}}]}}}`
	put := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/models/gateway", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	NewMgmtRouter().ServeHTTP(put, req)
	if put.Code != http.StatusOK {
		t.Fatalf("gateway apply status=%d body=%s", put.Code, put.Body.String())
	}

	preview := httptest.NewRecorder()
	NewMgmtRouter().ServeHTTP(preview, httptest.NewRequest(http.MethodGet, "/api/models/gateway/preview", nil))
	if preview.Code != http.StatusOK {
		t.Fatalf("gateway preview status=%d body=%s", preview.Code, preview.Body.String())
	}
	var response map[string]interface{}
	if err := json.Unmarshal(preview.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if pending, _ := response["pending_count"].(float64); pending != 0 {
		t.Fatalf("pending_count=%v after reload, want 0: %s", pending, preview.Body.String())
	}
	proposed := response["proposed"].(map[string]interface{})
	provider := proposed["pi-switch-chat"].(map[string]interface{})
	models := provider["models"].([]interface{})
	var edited map[string]interface{}
	for _, item := range models {
		model := item.(map[string]interface{})
		if model["id"] == "model-b" {
			edited = model
			break
		}
	}
	if edited == nil || edited["name"] != "Model B Browser Edited" {
		t.Fatalf("edited name was not canonicalized: %#v", edited)
	}
	if extra, _ := edited["extra"].(map[string]interface{}); extra["e2e"] != "browser" {
		t.Fatalf("edited extra was not preserved: %#v", edited["extra"])
	}
	if edited["contextWindow"] != float64(2000) || edited["maxTokens"] != float64(200) || edited["reasoning"] != true {
		t.Fatalf("edited model fields were not preserved: %#v", edited)
	}
	if input, _ := edited["input"].([]interface{}); len(input) != 2 || input[1] != "image" {
		t.Fatalf("edited input was not preserved: %#v", edited["input"])
	}
	if thinking, _ := edited["thinkingLevelMap"].(map[string]interface{}); thinking["high"] != "medium" {
		t.Fatalf("edited thinking levels were not preserved: %#v", edited["thinkingLevelMap"])
	}
	if cost, _ := edited["cost"].(map[string]interface{}); cost["input"] != 0.5 {
		t.Fatalf("edited cost was not preserved: %#v", edited["cost"])
	}
	if compat, _ := edited["compat"].(map[string]interface{}); compat["manual"] != true {
		t.Fatalf("edited compat was not preserved: %#v", edited["compat"])
	}
	if headers, _ := edited["headers"].(map[string]interface{}); headers["X-Model"] != "browser" {
		t.Fatalf("edited headers were not preserved: %#v", edited["headers"])
	}

	configBytes, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(configBytes), "Model B Browser Edited") {
		t.Fatalf("Gateway edit leaked into Supplier config: %s", configBytes)
	}
}
