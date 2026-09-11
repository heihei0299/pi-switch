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

// ARCH-03 / ARCH-05 收尾：显式 draft metadata 不得被 catalog 覆盖。
//
// draft 路径（preview 与 publish）只做 missing-only enrich：draft 对自己声明的
// 值拥有最终解释权，用户显式编辑的 contextWindow/maxTokens/reasoning/input/cost
// 必须原样进入 BuildDraftPlan。overwrite enrich 只属于 generated flow。
func TestGatewayDraft_ExplicitMetadataBeatsCatalog(t *testing.T) {
	dir := t.TempDir()
	// catalogFixture 的 test-model：context 5000 / output 500 / reasoning true /
	// input [text,image] / cost 1,2,0.1 —— 与下方 draft 显式值全部冲突。
	writeCatalogCache(t, dir)
	cfgJSON := `{"version":2,"profiles":{
		"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k",
			"upstreams":[{"name":"main","api":"openai-completions","baseUrl":"http://x","apiKey":"k",
			"models":[{"id":"test-model","contextWindow":0,"maxTokens":0}],
			"exposedModels":["test-model"]}]}},
		"settings":{"providerPrefix":"pi-switch"}}`
	writeChannelConfig(t, dir, cfgJSON)
	mp := filepath.Join(dir, "models.json")
	t.Setenv("PI_SWITCH_MODELS", mp)

	draft := map[string]interface{}{
		"providers": map[string]interface{}{
			"pi-switch-chat": map[string]interface{}{
				"api": "openai-completions", "baseUrl": "http://127.0.0.1:43112/v1",
				"apiKey": "pi-switch-proxy", "proxy": false,
				"models": []interface{}{map[string]interface{}{
					"id":            "test-model",
					"contextWindow": 111,
					"maxTokens":     11,
					"reasoning":     false,
					"input":         []string{"text"},
					"cost":          map[string]interface{}{"input": 9, "output": 9, "cacheRead": 9, "cacheWrite": 0},
				}},
			},
		},
	}
	body, err := json.Marshal(map[string]interface{}{"draft": draft})
	if err != nil {
		t.Fatal(err)
	}
	r := NewMgmtRouter()

	// preview：draft → enrich → BuildDraftPlan，返回值必须是 draft 自己的值。
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/models/gateway/preview", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("preview code = %d body %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	proposed, ok := resp["proposed"].(map[string]interface{})
	if !ok {
		t.Fatalf("preview proposed missing: %v", resp)
	}
	assertDraftMetadataWins(t, firstGatewayModel(t, proposed), "preview")

	// publish：PUT /api/models/gateway 落盘的 canonical proposed 保持同一结果。
	payload, _ := json.Marshal(draft)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("PUT", "/api/models/gateway", strings.NewReader(string(payload)))
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
	providers := stored["providers"].(map[string]interface{})
	assertDraftMetadataWins(t, firstGatewayModel(t, providers), "published")
}

func firstGatewayModel(t *testing.T, providers map[string]interface{}) map[string]interface{} {
	t.Helper()
	entry, ok := providers["pi-switch-chat"].(map[string]interface{})
	if !ok {
		t.Fatalf("pi-switch-chat provider missing: %v", providers)
	}
	models, _ := entry["models"].([]interface{})
	if len(models) != 1 {
		t.Fatalf("models = %v, want 1", models)
	}
	model, ok := models[0].(map[string]interface{})
	if !ok {
		t.Fatalf("model is not an object: %v", models[0])
	}
	return model
}

// assertDraftMetadataWins pins every field the report lists: an explicit draft value
// survives the catalog, while a field the draft omits is still filled from it.
func assertDraftMetadataWins(t *testing.T, m map[string]interface{}, where string) {
	t.Helper()
	if m["contextWindow"] != float64(111) {
		t.Fatalf("%s: contextWindow = %v, want 111 (catalog 5000 must not overwrite)", where, m["contextWindow"])
	}
	if m["maxTokens"] != float64(11) {
		t.Fatalf("%s: maxTokens = %v, want 11 (catalog 500 must not overwrite)", where, m["maxTokens"])
	}
	if m["reasoning"] != false {
		t.Fatalf("%s: reasoning = %v, want false (catalog true must not overwrite)", where, m["reasoning"])
	}
	input, _ := m["input"].([]interface{})
	if len(input) != 1 || input[0] != "text" {
		t.Fatalf("%s: input = %v, want [text] (catalog [text image] must not overwrite)", where, m["input"])
	}
	cost, ok := m["cost"].(map[string]interface{})
	if !ok || cost["input"] != float64(9) || cost["output"] != float64(9) || cost["cacheRead"] != float64(9) {
		t.Fatalf("%s: cost = %v, want the draft's 9/9/9", where, m["cost"])
	}
	// 补齐方向仍在：draft 未声明的 name 由 catalog 补上。
	if m["name"] != "Test Model" {
		t.Fatalf("%s: name = %v, want the catalog's Test Model (missing-only fill must still run)", where, m["name"])
	}
}
