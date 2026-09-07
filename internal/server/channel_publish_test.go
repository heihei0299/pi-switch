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

// S7: PUT /models/gateway 接收二次勾选子集（models 仅含选中条目）并原子写入网关文件；
// 未选中项落盘无残留，preview 对其保持 pending（可重做）。

func TestGatewayPublish_SubsetInjectsOnlySelected(t *testing.T) {
	dir := t.TempDir()
	cfgJSON := `{"version":2,"profiles":{
		"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","models":[],"upstreams":[
			{"name":"main","baseUrl":"http://a","apiKey":"k","models":[{"id":"m1","contextWindow":100,"maxTokens":10},{"id":"m2","contextWindow":100,"maxTokens":10}],"exposedModels":["m1","m2"]}]}},
		"settings":{"providerPrefix":"pi-switch","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112}}}`
	p := writeChannelConfig(t, dir, cfgJSON)
	mp := filepath.Join(dir, "models.json")
	t.Setenv("PI_SWITCH_MODELS", mp)
	_ = p
	r := NewMgmtRouter()

	payload := `{"providers":{"sup/main":{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","proxy":false,"models":[{"id":"m1","contextWindow":100,"maxTokens":10}]}}}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/api/models/gateway", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("subset publish code = %d body %s", w.Code, w.Body.String())
	}
	raw, err := os.ReadFile(mp)
	if err != nil {
		t.Fatalf("models.json not written: %v", err)
	}
	var stored map[string]interface{}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("decode: %v", err)
	}
	provs := stored["providers"].(map[string]interface{})
	entry := provs["sup/main"].(map[string]interface{})
	models := entry["models"].([]interface{})
	if len(models) != 1 {
		t.Fatalf("stored models = %v, want exactly [m1]", models)
	}
	if models[0].(map[string]interface{})["id"] != "m1" {
		t.Fatalf("stored models = %v, want [m1]", models)
	}
	// 未选中的 m2 在 preview 中保持 pending（可重做）
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/models/gateway/preview", nil)
	r.ServeHTTP(w2, req2)
	var preview map[string]interface{}
	_ = json.Unmarshal(w2.Body.Bytes(), &preview)
	if pc, _ := preview["pending_count"].(float64); pc == 0 {
		t.Fatalf("pending_count = 0, want >0 (m2 unselected)")
	}
}
