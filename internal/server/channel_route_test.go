package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// S9b: 三段 id 精确路由到渠道凭证：sup/bk/b1 只打 bk 渠道，
// 上游收到裸模型名；未暴露的三段 id 502。

func TestProxyRoute_ChannelPinnedID(t *testing.T) {
	dir := t.TempDir()
	var gotA, gotB []string
	mockA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotA = append(gotA, body["model"].(string))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"a","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer mockA.Close()
	mockB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotB = append(gotB, body["model"].(string))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"b","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer mockB.Close()
	cfgJSON := `{"version":2,"current":"sup","profiles":{
		"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","models":[],"upstreams":[
			{"name":"main","baseUrl":"` + mockA.URL + `","apiKey":"ka","models":[{"id":"m1","contextWindow":100,"maxTokens":10}],"exposedModels":["m1"]},
			{"name":"bk","baseUrl":"` + mockB.URL + `","apiKey":"kb","models":[{"id":"b1","contextWindow":200,"maxTokens":20}],"exposedModels":["b1"]}]}},
		"settings":{"providerPrefix":"pi-switch"}}`
	p := writeChannelConfig(t, dir, cfgJSON)
	_ = p
	r := NewProxyRouter()
	post := func(model string) int {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":`+jsonString(model)+`,"messages":[]}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w.Code
	}
	if code := post("sup/bk/b1"); code != 200 {
		t.Fatalf("pinned route code = %d, want 200", code)
	}
	if len(gotB) != 1 || gotB[0] != "b1" {
		t.Fatalf("bk upstream got %v, want [b1]", gotB)
	}
	if len(gotA) != 0 {
		t.Fatalf("main upstream got %v, want untouched", gotA)
	}
	if code := post("sup/main/m1"); code != 200 {
		t.Fatalf("main route code = %d, want 200", code)
	}
	// 未暴露的三段 id 不 pin，走既有 current 回退语义（与二段 id 一致）：
	// 去前缀后按裸名转发到主渠道。
	if code := post("sup/bk/ghost"); code != 200 {
		t.Fatalf("fallback route code = %d, want 200", code)
	}
	if len(gotA) == 0 || gotA[len(gotA)-1] != "bk/ghost" {
		t.Fatalf("fallback upstream got %v, want last bk/ghost", gotA)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
