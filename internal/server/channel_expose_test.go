package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// S4: PUT /api/profiles/:name/expose?channel=c 按渠道分区校验并写入该渠道暴露集；
// 未知渠道/未知 id 400；无参旧路径保持顶层语义。

func putExpose(t *testing.T, r interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, url, body string) int {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w.Code
}

func channelExposeConfig() string {
	return `{"version":2,"profiles":{"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","models":[],"upstreams":[{"name":"main","baseUrl":"http://a","apiKey":"k","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384},{"id":"shared","contextWindow":100,"maxTokens":10}],"exposedModels":[]},{"name":"bk","baseUrl":"http://b","apiKey":"k","models":[{"id":"b1","contextWindow":200,"maxTokens":20},{"id":"shared","contextWindow":300,"maxTokens":30}],"exposedModels":[]}]}},"settings":{"providerPrefix":"pi-switch"}}`
}

func TestChannelExpose_DirectedSetsChannelOnly(t *testing.T) {
	dir := t.TempDir()
	p := writeChannelConfig(t, dir, channelExposeConfig())
	r := NewMgmtRouter()
	if code := putExpose(t, r, "/api/profiles/sup/expose?channel=bk", `{"modelIds":["b1","shared"]}`); code != 200 {
		t.Fatalf("directed expose code = %d, want 200", code)
	}
	cfg := loadCfg(t, p)
	prof := cfg.Profiles["sup"]
	_, bkExposed := prof.ChannelView("bk")
	if len(bkExposed) != 2 || bkExposed[0] != "b1" || bkExposed[1] != "shared" {
		t.Fatalf("bk exposed = %v, want [b1 shared]", bkExposed)
	}
	_, mainExposed := prof.ChannelView("main")
	if len(mainExposed) != 0 {
		t.Fatalf("main exposed = %v, want empty (isolated)", mainExposed)
	}
	if len(prof.ExposedModels) != 0 {
		t.Fatalf("top-level exposed = %v, want untouched", prof.ExposedModels)
	}
}

func TestChannelExpose_RejectsUnknownIdAndChannel(t *testing.T) {
	dir := t.TempDir()
	writeChannelConfig(t, dir, channelExposeConfig())
	r := NewMgmtRouter()
	if code := putExpose(t, r, "/api/profiles/sup/expose?channel=bk", `{"modelIds":["m1"]}`); code != 400 {
		t.Fatalf("cross-channel id code = %d, want 400", code)
	}
	if code := putExpose(t, r, "/api/profiles/sup/expose?channel=nope", `{"modelIds":[]}`); code != 400 {
		t.Fatalf("unknown channel code = %d, want 400", code)
	}
	// 空集合法：表示不暴露
	if code := putExpose(t, r, "/api/profiles/sup/expose?channel=bk", `{"modelIds":[]}`); code != 200 {
		t.Fatalf("empty set code = %d, want 200", code)
	}
}

func TestChannelExpose_NoParamFallsBackToPrimaryChannel(t *testing.T) {
	dir := t.TempDir()
	p := writeChannelConfig(t, dir, channelExposeConfig())
	r := NewMgmtRouter()
	if code := putExpose(t, r, "/api/profiles/sup/expose", `{"modelIds":["m1"]}`); code != 200 {
		t.Fatalf("no-param expose code = %d, want 200", code)
	}
	cfg := loadCfg(t, p)
	prof := cfg.Profiles["sup"]
	_, mainExposed := prof.ChannelView("main")
	if len(mainExposed) != 1 || mainExposed[0] != "m1" {
		t.Fatalf("primary channel exposed = %v, want [m1]", mainExposed)
	}
}

func TestChannelExpose_LegacyTopLevelUnchanged(t *testing.T) {
	dir := t.TempDir()
	p := writeChannelConfig(t, dir, `{"version":2,"profiles":{"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384}],"exposedModels":[]}},"settings":{"providerPrefix":"pi-switch"}}`)
	r := NewMgmtRouter()
	if code := putExpose(t, r, "/api/profiles/sup/expose", `{"modelIds":["m1"]}`); code != 200 {
		t.Fatalf("legacy expose code = %d, want 200", code)
	}
	cfg := loadCfg(t, p)
	if got := cfg.Profiles["sup"].ExposedModels; len(got) != 1 || got[0] != "m1" {
		t.Fatalf("legacy top-level exposed = %v, want [m1]", got)
	}
	if code := putExpose(t, r, "/api/profiles/sup/expose", `{"modelIds":["ghost"]}`); code != 400 {
		t.Fatalf("legacy unknown id code = %d, want 400", code)
	}
}
