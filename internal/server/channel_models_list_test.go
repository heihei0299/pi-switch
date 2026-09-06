package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// S9: 代理 GET /v1/models 沿用已暴露聚合，id 保持前缀形态：
// 已分区 supplier/channel/modelId，未分区 supplier/modelId；空暴露不列出。

func TestProxyModels_ChannelPrefixedIDs(t *testing.T) {
	dir := t.TempDir()
	cfgJSON := `{"version":2,"current":"sup","profiles":{
		"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","models":[],"upstreams":[
			{"name":"main","baseUrl":"http://a","apiKey":"k","models":[{"id":"m1","contextWindow":100,"maxTokens":10}],"exposedModels":["m1"]},
			{"name":"bk","baseUrl":"http://b","apiKey":"k","models":[{"id":"m1","contextWindow":200,"maxTokens":20}],"exposedModels":[]}]},
		"leg":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://c","apiKey":"k","models":[{"id":"old","contextWindow":128000,"maxTokens":16384}],"exposedModels":["old"]},
		"empty":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://d","apiKey":"k","models":[{"id":"hid","contextWindow":100,"maxTokens":10}],"exposedModels":[]}},
		"settings":{"providerPrefix":"pi-switch"}}`
	p := writeChannelConfig(t, dir, cfgJSON)
	_ = p
	r := NewProxyRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/models", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("models code = %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := resp["data"].([]interface{})
	ids := map[string]bool{}
	for _, m := range data {
		ids[m.(map[string]interface{})["id"].(string)] = true
	}
	for _, want := range []string{"sup/main/m1", "leg/old", "sup/m1"} {
		if !ids[want] {
			t.Fatalf("ids = %v, want %q", ids, want)
		}
	}
	for _, absent := range []string{"sup/bk/m1", "empty/hid"} {
		if ids[absent] {
			t.Fatalf("ids = %v, must not contain %q", ids, absent)
		}
	}
}
