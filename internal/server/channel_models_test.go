package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// T8a: PUT /profiles/:name/models 支持 {channel, models} 写入渠道池；
// 未迁移 profile 先把顶层吸收进首渠道；未知渠道/dup id 400；
// validateProviderProfile 校验分区池dup 与暴露归属。

func putModels(t *testing.T, r interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, url, body string) int {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w.Code
}

func TestChannelModels_SavePoolAbsorbsTopLevel(t *testing.T) {
	dir := t.TempDir()
	p := writeChannelConfig(t, dir, `{"version":2,"profiles":{"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","models":[{"id":"top1","contextWindow":100,"maxTokens":10}],"exposedModels":["top1"],"upstreams":[{"name":"main","baseUrl":"http://a","apiKey":"k"},{"name":"bk","baseUrl":"http://b","apiKey":"k"}]}},"settings":{"providerPrefix":"pi-switch"}}`)
	r := NewMgmtRouter()
	body := `{"channel":"bk","models":[{"id":"b1","contextWindow":200,"maxTokens":20}]}`
	if code := putModels(t, r, "/api/profiles/sup/models", body); code != 200 {
		t.Fatalf("channel save code = %d, want 200", code)
	}
	cfg := loadCfg(t, p)
	prof := cfg.Profiles["sup"]
	mainModels, mainExposed := prof.ChannelView("main")
	if len(mainModels) != 1 || mainModels[0].ID != "top1" {
		t.Fatalf("main pool = %v, want absorbed [top1]", mainModels)
	}
	if len(mainExposed) != 1 || mainExposed[0] != "top1" {
		t.Fatalf("main exposed = %v, want absorbed [top1]", mainExposed)
	}
	bkModels, _ := prof.ChannelView("bk")
	if len(bkModels) != 1 || bkModels[0].ID != "b1" {
		t.Fatalf("bk pool = %v, want [b1]", bkModels)
	}
	if len(prof.Models) != 0 || len(prof.ExposedModels) != 0 {
		t.Fatalf("top-level not cleared: %v %v", prof.Models, prof.ExposedModels)
	}
}

func TestChannelModels_RejectsUnknownChannelAndDupIds(t *testing.T) {
	dir := t.TempDir()
	writeChannelConfig(t, dir, `{"version":2,"profiles":{"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","models":[],"upstreams":[{"name":"main","baseUrl":"http://a","apiKey":"k"}]}},"settings":{"providerPrefix":"pi-switch"}}`)
	r := NewMgmtRouter()
	if code := putModels(t, r, "/api/profiles/sup/models", `{"channel":"nope","models":[]}`); code != 400 {
		t.Fatalf("unknown channel code = %d, want 400", code)
	}
	dup := `{"channel":"main","models":[{"id":"x","contextWindow":100,"maxTokens":10},{"id":"x","contextWindow":100,"maxTokens":10}]}`
	if code := putModels(t, r, "/api/profiles/sup/models", dup); code != 400 {
		t.Fatalf("dup ids code = %d, want 400", code)
	}
}

func TestChannelModels_LegacyTopLevelUnchanged(t *testing.T) {
	dir := t.TempDir()
	p := writeChannelConfig(t, dir, `{"version":2,"profiles":{"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","models":[{"id":"m1","contextWindow":100,"maxTokens":10}]}},"settings":{"providerPrefix":"pi-switch"}}`)
	r := NewMgmtRouter()
	body := `{"models":[{"id":"m1","contextWindow":100,"maxTokens":10},{"id":"m2","contextWindow":100,"maxTokens":10}]}`
	if code := putModels(t, r, "/api/profiles/sup/models", body); code != 200 {
		t.Fatalf("legacy save code = %d, want 200", code)
	}
	cfg := loadCfg(t, p)
	if len(cfg.Profiles["sup"].Models) != 2 {
		t.Fatalf("legacy models = %v, want 2", cfg.Profiles["sup"].Models)
	}
}

func TestChannelValidation_PartitionPoolsChecked(t *testing.T) {
	// dup within channel pool → 400
	dup := `{"name":"pp","profile":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://a","apiKey":"k","models":[],"upstreams":[{"name":"c1","baseUrl":"http://a","apiKey":"k","models":[{"id":"x"},{"id":"x"}]}]}}`
	if code := postProfile(t, dup); code != 400 {
		t.Fatalf("pool dup code = %d, want 400", code)
	}
	// exposed not in pool → 400
	badExp := `{"name":"pp","profile":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://a","apiKey":"k","models":[],"upstreams":[{"name":"c1","baseUrl":"http://a","apiKey":"k","models":[{"id":"x"}],"exposedModels":["ghost"]}]}}`
	if code := postProfile(t, badExp); code != 400 {
		t.Fatalf("pool expose-unknown code = %d, want 400", code)
	}
	// valid partitioned → 200
	ok := `{"name":"pp","profile":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://a","apiKey":"k","models":[],"upstreams":[{"name":"c1","baseUrl":"http://a","apiKey":"k","models":[{"id":"x"}],"exposedModels":["x"]}]}}`
	if code := postProfile(t, ok); code != 200 {
		t.Fatalf("valid partitioned code = %d, want 200", code)
	}
}
