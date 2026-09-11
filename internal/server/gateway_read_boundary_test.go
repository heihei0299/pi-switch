package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// 审查报告 P2（system-contract §2.3）：models.json 的读取语义只有 gateway.ReadCurrent
// 一个边界。文件不存在 → 空 current 且 200；不可读/JSON 损坏 → 500 报错，
// 不再由 server 自己发明 `gateway: null` 吞掉错误。
func TestGatewayGet_ReadBoundary(t *testing.T) {
	dir := t.TempDir()
	writeChannelConfig(t, dir, `{"version":2,"profiles":{},"settings":{"providerPrefix":"pi-switch"}}`)
	modelsPath := filepath.Join(dir, "models.json")
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	r := NewMgmtRouter()

	get := func() *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/models/gateway", nil))
		return w
	}
	var envelope struct {
		Gateway *struct {
			Providers map[string]interface{} `json:"providers"`
		} `json:"gateway"`
	}

	// 未发布：空 current，而不是 null。
	w := get()
	if w.Code != http.StatusOK {
		t.Fatalf("GET gateway without models.json = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode gateway: %v (%s)", err, w.Body.String())
	}
	if envelope.Gateway == nil || envelope.Gateway.Providers == nil {
		t.Fatalf("GET gateway without models.json = %s, want {\"providers\":{}} and never null", w.Body.String())
	}

	// 损坏：读取边界报错，管理面仍是字符串 envelope。
	if err := os.WriteFile(modelsPath, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	w = get()
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("GET gateway with corrupt models.json = %d, want 500 (body=%s)", w.Code, w.Body.String())
	}
	var errBody struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &errBody); err != nil || errBody.Error == "" {
		t.Fatalf("GET gateway management envelope = %s, want {\"error\":\"...\"}", w.Body.String())
	}

	// 正常：provider 原样出现在 current 里。
	if err := os.WriteFile(modelsPath, []byte(`{"providers":{"pi-switch-chat":{"api":"openai-completions","models":[]}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	w = get()
	if w.Code != http.StatusOK {
		t.Fatalf("GET gateway with models.json = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode gateway: %v (%s)", err, w.Body.String())
	}
	if envelope.Gateway == nil {
		t.Fatalf("current is null: %s", w.Body.String())
	}
	if _, ok := envelope.Gateway.Providers["pi-switch-chat"]; !ok {
		t.Fatalf("current lost the published provider: %s", w.Body.String())
	}
}
