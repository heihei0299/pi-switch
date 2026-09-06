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

// S2: preview 对提议条目做目录补缺并返回 enrich 摘要。
// fixture 缓存（新鲜 mtime）→ 不碰网络。

const catalogFixture = `{"testlab":{"models":{
  "test-model":{"name":"Test Model","reasoning":true,
    "modalities":{"input":["text","image"]},
    "limit":{"context":5000,"output":500},
    "cost":{"input":1,"output":2,"cache_read":0.1}},
  "bare-only":{"name":"Bare","limit":{"context":100,"output":10},
    "cost":{"input":0.5,"output":1,"cache_read":0.05}}
}}}`

func writeCatalogCache(t *testing.T, dir string) {
	t.Helper()
	p := filepath.Join(dir, "models-dev.json")
	if err := os.WriteFile(p, []byte(catalogFixture), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CATALOG", p)
}

func TestGatewayPreview_CatalogFillsMissing(t *testing.T) {
	dir := t.TempDir()
	writeCatalogCache(t, dir)
	cfgJSON := `{"version":2,"profiles":{
		"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k",
			"models":[{"id":"test-model","contextWindow":0,"maxTokens":0}],
			"exposedModels":["test-model"]}},
		"settings":{"providerPrefix":"pi-switch"}}`
	writeChannelConfig(t, dir, cfgJSON)
	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/models/gateway/preview", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("preview code = %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	prop := resp["proposed"].(map[string]interface{})
	models := prop["models"].([]interface{})
	if len(models) != 1 {
		t.Fatalf("models = %v", models)
	}
	m := models[0].(map[string]interface{})
	if m["contextWindow"] != float64(5000) || m["maxTokens"] != float64(500) {
		t.Fatalf("limits not filled: %v", m)
	}
	if m["name"] != "Test Model" {
		t.Fatalf("name not filled: %v", m)
	}
	cost, ok := m["cost"].(map[string]interface{})
	if !ok || cost["input"] != float64(1) || cost["cacheRead"] != float64(0.1) {
		t.Fatalf("cost not filled: %v", m["cost"])
	}
	if _, ok := cost["cacheWrite"]; !ok {
		t.Fatalf("cost lacks cacheWrite: %v", cost)
	}
	enrich, ok := resp["enrich"].(map[string]interface{})
	if !ok {
		t.Fatalf("enrich summary missing: %v", resp)
	}
	if enrich["enriched"] != float64(1) {
		t.Fatalf("enrich = %v, want enriched=1", enrich)
	}
}

func TestGatewayPreview_CatalogUnmatchedSkipped(t *testing.T) {
	dir := t.TempDir()
	writeCatalogCache(t, dir)
	cfgJSON := `{"version":2,"profiles":{
		"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k",
			"models":[{"id":"ghost-model","contextWindow":100,"maxTokens":10}],
			"exposedModels":["ghost-model"]}},
		"settings":{"providerPrefix":"pi-switch"}}`
	writeChannelConfig(t, dir, cfgJSON)
	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/models/gateway/preview", nil)
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	prop := resp["proposed"].(map[string]interface{})
	m := prop["models"].([]interface{})[0].(map[string]interface{})
	if _, ok := m["cost"]; ok {
		t.Fatalf("unmatched model must not gain cost: %v", m)
	}
	enrich, ok := resp["enrich"].(map[string]interface{})
	if !ok {
		t.Fatalf("enrich summary missing")
	}
	if enrich["enriched"] != float64(0) || enrich["skipped"] != float64(1) {
		t.Fatalf("enrich = %v, want enriched=0 skipped=1", enrich)
	}
}

func TestGatewayPublish_CatalogFillsBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	writeCatalogCache(t, dir)
	cfgJSON := `{"version":2,"profiles":{
		"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k",
			"models":[{"id":"test-model","contextWindow":0,"maxTokens":0}],
			"exposedModels":["test-model"]}},
		"settings":{"providerPrefix":"pi-switch"}}`
	writeChannelConfig(t, dir, cfgJSON)
	mp := filepath.Join(dir, "models.json")
	t.Setenv("PI_SWITCH_MODELS", mp)
	r := NewMgmtRouter()
	payload := `{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","proxy":false,"models":[{"id":"sup/test-model"}]}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/models/gateway", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("publish code = %d body %s", w.Code, w.Body.String())
	}
	raw, err := os.ReadFile(mp)
	if err != nil {
		t.Fatalf("models.json not written: %v", err)
	}
	var stored map[string]interface{}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	entry := stored["providers"].(map[string]interface{})["pi-switch"].(map[string]interface{})
	models := entry["models"].([]interface{})
	if len(models) != 1 {
		t.Fatalf("stored models = %v", models)
	}
	m := models[0].(map[string]interface{})
	cost, ok := m["cost"].(map[string]interface{})
	if !ok || cost["input"] != float64(1) {
		t.Fatalf("stored cost not enriched: %v", m)
	}
	if m["contextWindow"] != float64(5000) {
		t.Fatalf("stored limits not enriched: %v", m)
	}
}

func TestGatewayRoutePublish_CatalogFillsBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	writeCatalogCache(t, dir)
	cfgJSON := `{"version":2,"profiles":{
		"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k",
			"models":[{"id":"test-model","contextWindow":0,"maxTokens":0}],
			"exposedModels":["test-model"]}},
		"settings":{"providerPrefix":"pi-switch"}}`
	writeChannelConfig(t, dir, cfgJSON)
	mp := filepath.Join(dir, "models.json")
	t.Setenv("PI_SWITCH_MODELS", mp)
	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/gateway/publish", strings.NewReader(""))
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("route publish code = %d body %s", w.Code, w.Body.String())
	}
	raw, err := os.ReadFile(mp)
	if err != nil {
		t.Fatalf("models.json not written: %v", err)
	}
	var stored map[string]interface{}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	entry := stored["providers"].(map[string]interface{})["pi-switch"].(map[string]interface{})
	m := entry["models"].([]interface{})[0].(map[string]interface{})
	if m["contextWindow"] != float64(5000) {
		t.Fatalf("route-published limits not enriched: %v", m)
	}
	if _, ok := m["cost"].(map[string]interface{}); !ok {
		t.Fatalf("route-published cost not enriched: %v", m)
	}
}

func TestGatewayPut_WrapperEnrichedSameAsSingle(t *testing.T) {
	dir := t.TempDir()
	writeCatalogCache(t, dir)
	cfgJSON := `{"version":2,"profiles":{
		"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k",
			"models":[{"id":"test-model","contextWindow":0,"maxTokens":0}],
			"exposedModels":["test-model"]}},
		"settings":{"providerPrefix":"pi-switch"}}`
	writeChannelConfig(t, dir, cfgJSON)
	mp := filepath.Join(dir, "models.json")
	t.Setenv("PI_SWITCH_MODELS", mp)
	r := NewMgmtRouter()
	// providers wrapper 经 PUT 写入：内层条目须与单条目同一口径被补齐。
	payload := `{"providers":{"pi-switch":{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","proxy":false,"models":[{"id":"sup/test-model"}]}}}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/models/gateway", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("wrapper put code = %d body %s", w.Code, w.Body.String())
	}
	raw, err := os.ReadFile(mp)
	if err != nil {
		t.Fatalf("models.json not written: %v", err)
	}
	var stored map[string]interface{}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	// 写入路径原语义不变（Publish 按单条目落盘）；断言补齐发生在某层即可。
	found := false
	var walk func(v interface{})
	walk = func(v interface{}) {
		switch vv := v.(type) {
		case map[string]interface{}:
			if vv["id"] == "sup/test-model" {
				if cost, ok := vv["cost"].(map[string]interface{}); ok && cost["input"] == float64(1) {
					found = true
				}
			}
			for _, c := range vv {
				walk(c)
			}
		case []interface{}:
			for _, c := range vv {
				walk(c)
			}
		}
	}
	walk(stored)
	if !found {
		t.Fatalf("wrapper inner model not enriched: %s", string(raw))
	}
}
