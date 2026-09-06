package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchEnrich_DeepseekEnriched3(t *testing.T) {
	dir := t.TempDir()
	catData := `{"deepseek": {"models": {"deepseek/deepseek-v4-flash": {"name": "Flash", "limit": {"context": 1048576, "output": 384000}, "cost": {"input": 0.14, "output": 0.28, "cache_read": 0.028}}, "deepseek/deepseek-v4-pro": {"name": "Pro", "limit": {"context": 1048576, "output": 384000}, "cost": {"input": 0.435, "output": 0.87, "cache_read": 0.003}}, "deepseek/deepseek-v3.2": {"name": "V32", "limit": {"context": 128000, "output": 64000}, "cost": {"input": 0.2174, "output": 0.326, "cache_read": 0.06}}}}}`
	p := filepath.Join(dir, "models-dev.json")
	if err := os.WriteFile(p, []byte(catData), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CATALOG", p)

	mock := channelMock(t, "deepseek-v4-flash", "deepseek-v4-pro", "deepseek-v3.2")
	defer mock.Close()

	cfgJSON := `{"version":2,"profiles":{"deepseek":{"api":"openai-completions","responsesMode":"auto","baseUrl":"` + mock.URL + `","apiKey":"sk-deep","preset":"deepseek","modelsDevProvider":"deepseek","upstreams":[{"name":"main","baseUrl":"` + mock.URL + `","apiKey":"sk-deep"}]}},"settings":{"providerPrefix":"pi-switch"}}`
	cfgPath := writeChannelConfig(t, dir, cfgJSON)
	t.Setenv("PI_SWITCH_MODELS", filepath.Join(dir, "models.json"))

	r := NewMgmtRouter()
	code, resp := postFetch(t, r, "/api/profiles/deepseek/fetch-models?channel=main")
	if code != 200 {
		t.Fatalf("fetch code %d resp %v", code, resp)
	}
	enrich, _ := resp["enrich"].(map[string]interface{})
	if enrich == nil {
		t.Fatalf("enrich missing %v", resp)
	}
	if int(enrich["enriched"].(float64)) != 3 {
		t.Fatalf("enriched = %v want 3, resp %v", enrich["enriched"], resp)
	}
	// verify config persisted with 1048576
	b, _ := os.ReadFile(cfgPath)
	var cfgMap map[string]interface{}
	_ = json.Unmarshal(b, &cfgMap)
	prof := cfgMap["profiles"].(map[string]interface{})["deepseek"].(map[string]interface{})
	upstreams, ok := prof["upstreams"].([]interface{})
	if !ok || len(upstreams)==0 {
		t.Fatalf("upstreams missing %v", prof)
	}
	first := upstreams[0].(map[string]interface{})
	models, _ := first["models"].([]interface{})
	if len(models) != 3 {
		t.Fatalf("persisted models len %d want 3, %v", len(models), models)
	}
	foundFlash := false
	for _, m := range models {
		mm := m.(map[string]interface{})
		if mm["id"] == "deepseek-v4-flash" {
			foundFlash = true
			if mm["contextWindow"] != float64(1048576) {
				t.Fatalf("persisted flash contextWindow %v want 1048576", mm["contextWindow"])
			}
			if mm["maxTokens"] != float64(384000) {
				t.Fatalf("persisted flash maxTokens %v want 384000", mm["maxTokens"])
			}
		}
	}
	if !foundFlash {
		t.Fatalf("deepseek-v4-flash not persisted")
	}
}
