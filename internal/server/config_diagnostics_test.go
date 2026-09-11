package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 后续审查 P2（a）：整文件门必须按每个 channel 的 effective api/mode 判定，否则能存下
// 一个运行期必然被 translator.PlanRequest 拒绝的配置（channel 显式 api + 不兼容 mode）。
func TestPutConfig_ChecksChannelEffectiveResponsesMode(t *testing.T) {
	isolateConfig(t)
	r := NewMgmtRouter()

	put := func(profiles string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{"version":2,"profiles":`+profiles+`}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}

	// channel 自带 openai-completions 却要求 passthrough：请求期必失败 → 400。
	w := put(`{"p":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://example.test/v1","apiKey":"k","upstreams":[{"name":"main","baseUrl":"https://example.test/v1","apiKey":"k","api":"openai-completions","responsesMode":"passthrough","models":[{"id":"m1"}]}]}}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PUT /api/config with a channel-only mismatch = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
	if body := w.Body.String(); !strings.Contains(body, "upstreams[0]") || !strings.Contains(body, "passthrough") {
		t.Fatalf("rejection must name the channel and the rule: %s", body)
	}

	// channel 未声明 api：回退 profile api（运行期同样如此），不得因此拒绝保存。
	w = put(`{"p":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://example.test/v1","apiKey":"k","upstreams":[{"name":"main","baseUrl":"https://example.test/v1","apiKey":"k","models":[{"id":"m1"}]}]}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/config with an api-less channel = %d, want 200 (the door must not be stricter than the runtime): %s", w.Code, w.Body.String())
	}
}

// 后续审查 P2（b）：宽松保存必须配完整 advisory validation —— shape/channel/model
// 诊断整份来自 profile.ProfileIssues，且逐条带字段路径。
func TestValidate_ReportsProfileShapeIssues(t *testing.T) {
	isolateConfig(t)
	// 一个 profile 同时踩多种 shape 问题：channel baseUrl 无 scheme、重复 channel 名、
	// 模型 id 为空、exposedModels 指向池外模型。
	cfg := `{"version":2,"profiles":{"p":{
		"api":"openai-completions","responsesMode":"auto","baseUrl":"https://example.test/v1","apiKey":"k",
		"upstreams":[
			{"name":"main","baseUrl":"ftp://example.test","apiKey":"k","api":"openai-completions","models":[{"id":""},{"id":"m1"},{"id":"m1"}],"exposedModels":["ghost"]},
			{"name":"main","baseUrl":"https://example.test/v1","apiKey":"k","api":"openai-completions","models":[{"id":"m2"}]}
		]}}}
	`
	writeChannelConfig(t, t.TempDir(), cfg)
	r := NewMgmtRouter()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/config/validate", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/config/validate = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	var issues []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &issues); err != nil {
		t.Fatalf("decode issues: %v (%s)", err, w.Body.String())
	}
	paths := map[string]string{}
	for _, issue := range issues {
		path, _ := issue["path"].(string)
		message, _ := issue["message"].(string)
		paths[path] = message
	}
	// 每一种 shape 问题都必须以真实字段路径被报出来（不止第一条）。
	for _, want := range []string{
		"profiles.p.upstreams[0].baseUrl",
		"profiles.p.upstreams[0].models[0].id",
		"profiles.p.upstreams[0].models[2].id",
		"profiles.p.upstreams[0].exposedModels[0]",
		"profiles.p.upstreams[1].name",
	} {
		if _, ok := paths[want]; !ok {
			t.Fatalf("missing issue for %s; got %v", want, paths)
		}
	}
	if msg := paths["profiles.p.upstreams[1].name"]; !strings.Contains(msg, "duplicate upstream name") {
		t.Fatalf("duplicate channel message = %q", msg)
	}
}
