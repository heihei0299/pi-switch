package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGatewayEnrich_OverwriteDefault(t *testing.T) {
	dir := t.TempDir()
	// catalog with 1048576 entry for deepseek-v4-flash under deepseek provider (and tokengo duplicate to force ambiguity)
	catData := `{"deepseek": {"models": {"deepseek/deepseek-v4-flash": {"name": "Flash", "reasoning": true, "modalities": {"input": ["text"]}, "limit": {"context": 1048576, "output": 384000}, "cost": {"input": 0.14, "output": 0.28, "cache_read": 0.028}}}}, "tokengo": {"models": {"deepseek/deepseek-v4-flash": {"name": "FlashT", "limit": {"context": 200, "output": 20}, "cost": {"input": 0.098, "output": 0.196, "cache_read": 0.028}}}}}`
	p := filepath.Join(dir, "models-dev.json")
	if err := os.WriteFile(p, []byte(catData), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CATALOG", p)
	cfgJSON := `{"version":2,"profiles":{"deepseek":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","models":[{"id":"deepseek-v4-flash","contextWindow":128000,"maxTokens":16384,"cost":{"input":0,"output":0,"cacheRead":0}}],"exposedModels":["deepseek-v4-flash"]}},"settings":{"providerPrefix":"pi-switch"}}`
	writeChannelConfig(t, dir, cfgJSON)
	t.Setenv("PI_SWITCH_MODELS", filepath.Join(dir, "models.json"))
	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/models/gateway/preview", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("preview code %d body %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	prop, ok := resp["proposed"].(map[string]interface{})
	if !ok {
		t.Fatalf("proposed missing")
	}
	models, _ := prop["models"].([]interface{})
	if len(models) != 1 {
		t.Fatalf("models len %d want 1", len(models))
	}
	m := models[0].(map[string]interface{})
	if m["contextWindow"] != float64(1048576) {
		t.Fatalf("contextWindow = %v want 1048576 (overwrite 128000)", m["contextWindow"])
	}
	if m["maxTokens"] != float64(384000) {
		t.Fatalf("maxTokens = %v want 384000", m["maxTokens"])
	}
	// enriched should be 1
	enrich, _ := resp["enrich"].(map[string]interface{})
	if enrich["enriched"] != float64(1) {
		t.Fatalf("enrich enriched = %v want 1, %v", enrich["enriched"], enrich)
	}
}
