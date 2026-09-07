package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// S6: GET /api/models/gateway/preview 返回按供应商/渠道分组的已暴露候选，
// 每模型带 published/pending 状态；current 独有进 removed；既有字段保持兼容。

func TestGatewayPreview_GroupsBySupplierChannel(t *testing.T) {
	dir := t.TempDir()
	cfgJSON := `{"version":2,"profiles":{
		"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","models":[],"upstreams":[
			{"name":"main","api":"openai-completions","baseUrl":"http://a","apiKey":"k","models":[{"id":"m1","contextWindow":100,"maxTokens":10}],"exposedModels":["m1"]},
			{"name":"bk","api":"openai-completions","baseUrl":"http://b","apiKey":"k","models":[{"id":"b1","contextWindow":200,"maxTokens":20}],"exposedModels":["b1"]}]},
		"leg":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://c","apiKey":"k","upstreams":[{"name":"main","api":"openai-completions","baseUrl":"http://c","apiKey":"k","models":[{"id":"old","contextWindow":128000,"maxTokens":16384}],"exposedModels":["old"]}]}},
		"settings":{"providerPrefix":"pi-switch"}}`
	p := writeChannelConfig(t, dir, cfgJSON)
	modelsJSON := `{"providers":{"sup/main":{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","proxy":false,"models":[{"id":"m1","contextWindow":100,"maxTokens":10}]},"leg/main":{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","proxy":false,"models":[{"id":"old","contextWindow":128000,"maxTokens":16384}]},"legacy":{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","proxy":false,"models":[{"id":"ghost/x","contextWindow":10,"maxTokens":10}]}}}`
	mp := filepath.Join(dir, "models.json")
	if err := os.WriteFile(mp, []byte(modelsJSON), 0644); err != nil {
		t.Fatalf("write models: %v", err)
	}
	t.Setenv("PI_SWITCH_MODELS", mp)
	_ = p
	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/models/gateway/preview", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("preview code = %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// 既有字段兼容
	if _, ok := resp["current"]; !ok {
		t.Fatalf("current missing: %v", resp)
	}
	if _, ok := resp["proposed"]; !ok {
		t.Fatalf("proposed missing")
	}
	// 分组断言
	groups, ok := resp["groups"].([]interface{})
	if !ok {
		t.Fatalf("groups missing: %v", resp)
	}
	got := map[string]string{}
	for _, g := range groups {
		gm := g.(map[string]interface{})
		key := gm["supplier"].(string) + "/" + gm["channel"].(string)
		for _, m := range gm["models"].([]interface{}) {
			mm := m.(map[string]interface{})
			got[key+"/"+mm["id"].(string)] = mm["status"].(string)
		}
	}
	want := map[string]string{
		"sup/main/m1":  "published",
		"sup/bk/b1":    "pending",
		"leg/main/old": "published",
	}
	for k, ws := range want {
		if got[k] != ws {
			t.Fatalf("status %q = %q, want %q (all=%v)", k, got[k], ws, got)
		}
	}
	// removed
	removed, _ := resp["removed"].([]interface{})
	if len(removed) != 1 || removed[0].(string) != "legacy/ghost/x" {
		t.Fatalf("removed = %v, want [legacy/ghost/x]", removed)
	}
}
