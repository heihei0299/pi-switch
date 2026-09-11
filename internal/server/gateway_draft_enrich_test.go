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

// The provider key the draft uses; the plan and the payload must name the same one.
//
// Deliberately pinned as a literal instead of derived from
// gateway.FixedGatewayProviders(): that list is exported for the WebUI, while a Go
// test here has to fail loudly if the published key is renamed, not follow it.
const draftGatewayProvider = "pi-switch-chat"

// Every catalog field conflicts with the draft values used below, so any field that
// keeps its draft value proves the catalog did not overwrite it.
const draftEnrichCatalog = `{"acme":{"models":{"flash-x":{
	"name":"Flash X","reasoning":true,
	"modalities":{"input":["text","image"]},
	"limit":{"context":1048576,"output":131072},
	"cost":{"input":0.5,"output":1.5,"cache_read":0.1}}}}}`

// draftEnrichFixture points both one-shot lookups at temp files: a catalog whose
// metadata differs from every draft value, and a config exposing one catalog model.
func draftEnrichFixture(t *testing.T, dir string) string {
	t.Helper()
	catalogPath := filepath.Join(dir, "models-dev.json")
	if err := os.WriteFile(catalogPath, []byte(draftEnrichCatalog), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CATALOG", catalogPath)
	cfgJSON := `{"version":2,"profiles":{
		"acme":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k",
			"upstreams":[{"name":"main","api":"openai-completions","baseUrl":"http://x","apiKey":"k",
			"models":[{"id":"flash-x","contextWindow":0,"maxTokens":0}],
			"exposedModels":["flash-x"]}]}},
		"settings":{"providerPrefix":"pi-switch"}}`
	writeChannelConfig(t, dir, cfgJSON)
	modelsPath := filepath.Join(dir, "models.json")
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	return modelsPath
}

// publishDraft PUTs a hand-edited draft and returns the providers that landed in
// models.json — the canonical plan the publish wrote.
func publishDraft(t *testing.T, r http.Handler, draft map[string]interface{}) map[string]interface{} {
	t.Helper()
	payload, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/models/gateway", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("publish code = %d body %s", w.Code, w.Body.String())
	}
	raw, err := os.ReadFile(os.Getenv("PI_SWITCH_MODELS"))
	if err != nil {
		t.Fatalf("models.json not written: %v", err)
	}
	var stored map[string]interface{}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	providers, ok := stored["providers"].(map[string]interface{})
	if !ok {
		t.Fatalf("published providers = %v", stored["providers"])
	}
	return providers
}

func draftModel(fields map[string]interface{}) map[string]interface{} {
	model := map[string]interface{}{"id": "flash-x"}
	for key, value := range fields {
		model[key] = value
	}
	return map[string]interface{}{
		"providers": map[string]interface{}{
			draftGatewayProvider: map[string]interface{}{
				"api": "openai-completions", "baseUrl": "http://127.0.0.1:43112/v1",
				"apiKey": "pi-switch-proxy", "proxy": false,
				"models": []interface{}{model},
			},
		},
	}
}

// ARCH-03 / ARCH-05 收尾（审查报告 P1）：显式 draft metadata 不得被 catalog 覆盖。
//
// 数值取自报告的指定场景：catalog contextWindow = 1048576，draft contextWindow = 111，
// 最终 canonical draft 必须保持 111，maxTokens / reasoning / input / cost 同理。
// Generated 路径仍用 FillOverwrite；draft 路径只补 draft 未声明的字段。
func TestGatewayDraft_ExplicitMetadataBeatsCatalog(t *testing.T) {
	dir := t.TempDir()
	draftEnrichFixture(t, dir)
	draft := draftModel(map[string]interface{}{
		"contextWindow": 111,
		"maxTokens":     11,
		"reasoning":     false,
		"input":         []string{"text"},
		"cost":          map[string]interface{}{"input": 9, "output": 9, "cacheRead": 9, "cacheWrite": 0},
	})
	body, err := json.Marshal(map[string]interface{}{"draft": draft})
	if err != nil {
		t.Fatal(err)
	}
	r := NewMgmtRouter()

	// preview：draft → enrich → BuildDraftPlan，返回值必须是 draft 自己的值。
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/models/gateway/preview", strings.NewReader(string(body)))
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

	// 对照（models.json 尚未发布，current 为空）：Generated 路径仍把 config 里的
	// 陈旧默认值刷成 catalog 值，overwrite enrich 未被削弱。
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/models/gateway/preview", nil))
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	generated, ok := resp["proposed"].(map[string]interface{})
	if !ok {
		t.Fatalf("generated proposed missing: %v", resp)
	}
	if got := firstGatewayModel(t, generated)["contextWindow"]; got != float64(1048576) {
		t.Fatalf("generated contextWindow = %v, want the catalog's 1048576", got)
	}

	// publish：落盘的 canonical proposed 保持同一结果。
	assertDraftMetadataWins(t, firstGatewayModel(t, publishDraft(t, r, draft)), "published")
}

// 审查后的补丁：draft 显式写下的 cost 0 是已知零价（免费模型），不是空缺。
// 覆盖检查按子字段判「键是否存在」——缺键才补，写下的 0 必须活下来。
func TestGatewayDraft_StatedZeroCostIsAFreePrice(t *testing.T) {
	dir := t.TempDir()
	draftEnrichFixture(t, dir)
	r := NewMgmtRouter()

	free := publishDraft(t, r, draftModel(map[string]interface{}{
		"cost": map[string]interface{}{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0},
	}))
	cost, ok := firstGatewayModel(t, free)["cost"].(map[string]interface{})
	if !ok {
		t.Fatalf("cost missing: %v", firstGatewayModel(t, free))
	}
	for _, field := range []string{"input", "output", "cacheRead"} {
		if cost[field] != float64(0) {
			t.Fatalf("cost.%s = %v, want the stated 0 (catalog prices must not overwrite a free price)", field, cost[field])
		}
	}

	// 缺键仍是空缺：不写 cost 的 draft 照旧从目录拿到价格。
	enriched := publishDraft(t, r, draftModel(map[string]interface{}{}))
	cost, ok = firstGatewayModel(t, enriched)["cost"].(map[string]interface{})
	if !ok {
		t.Fatalf("cost missing: %v", firstGatewayModel(t, enriched))
	}
	if cost["input"] != float64(0.5) || cost["cacheRead"] != float64(0.1) {
		t.Fatalf("cost = %v, want the catalog's 0.5/0.1 (an absent key is a gap)", cost)
	}
}

// 后续审查 P2：显式清空 name 是「没有名字」，不是「没写 name」。
// 键存在即已声明——写下的空串必须活下来，否则 catalog 会把用户清掉的显示名写回去。
func TestGatewayDraft_StatedEmptyNameIsNotRefilled(t *testing.T) {
	dir := t.TempDir()
	draftEnrichFixture(t, dir)
	r := NewMgmtRouter()

	cleared := publishDraft(t, r, draftModel(map[string]interface{}{"name": ""}))
	if name := firstGatewayModel(t, cleared)["name"]; name != "" {
		t.Fatalf("name = %v, want the stated empty string (catalog's Flash X must not be written back)", name)
	}

	// 缺键仍是空缺：不写 name 的 draft 照旧从目录拿到显示名。
	filled := publishDraft(t, r, draftModel(map[string]interface{}{}))
	if name := firstGatewayModel(t, filled)["name"]; name != "Flash X" {
		t.Fatalf("name = %v, want the catalog's Flash X (an absent key is a gap)", name)
	}
}

func firstGatewayModel(t *testing.T, providers map[string]interface{}) map[string]interface{} {
	t.Helper()
	entry, ok := providers[draftGatewayProvider].(map[string]interface{})
	if !ok {
		t.Fatalf("%s provider missing: %v", draftGatewayProvider, providers)
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
		t.Fatalf("%s: contextWindow = %v, want 111 (catalog 1048576 must not overwrite)", where, m["contextWindow"])
	}
	if m["maxTokens"] != float64(11) {
		t.Fatalf("%s: maxTokens = %v, want 11 (catalog 131072 must not overwrite)", where, m["maxTokens"])
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
	if m["name"] != "Flash X" {
		t.Fatalf("%s: name = %v, want the catalog's Flash X (missing-only fill must still run)", where, m["name"])
	}
}
