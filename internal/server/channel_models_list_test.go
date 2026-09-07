package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// S9: 代理 GET /v1/models 沿用已暴露聚合，id 保持前缀形态：
// 已分区 supplier/channel/modelId，未分区 supplier/modelId；空暴露不列出。

// S9: 代理 GET /v1/models 按 Channel 分组返回裸 id，owned_by 为 providerKey
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
	// New per-channel bare: id is bare, owned_by is providerKey
	got := map[string]string{} // id -> owned_by
	for _, m := range data {
		mm := m.(map[string]interface{})
		got[mm["id"].(string)] = mm["owned_by"].(string)
	}
	if got["m1"] != "sup/main" {
		t.Fatalf("m1 owned_by = %q want sup/main, got %v", got["m1"], got)
	}
	if got["old"] != "leg" {
		t.Fatalf("old owned_by = %q want leg, got %v", got["old"], got)
	}
	if _, ok := got["hid"]; ok {
		t.Fatalf("empty/hid must not be exposed, got %v", got)
	}
	// Ensure no prefixed ids
	for _, m := range data {
		id := m.(map[string]interface{})["id"].(string)
		if id == "sup/main/m1" || id == "leg/old" || id == "sup/m1" {
			t.Fatalf("should not have prefixed/short id %q, got bare only", id)
		}
	}
}
