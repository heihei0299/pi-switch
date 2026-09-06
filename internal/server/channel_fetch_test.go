package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

// S3: POST /api/profiles/:name/fetch-models?channel=c 定向渠道拉取，enrich 后合并入
// 该渠道池并落盘；未知渠道 400；单渠道失败不污染其他渠道；旧无参保持原行为。

func channelMock(t *testing.T, ids ...string) *httptest.Server {
	t.Helper()
	data := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		data = append(data, map[string]interface{}{"id": id})
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
	}))
}

func writeChannelConfig(t *testing.T, dir, cfg string) string {
	t.Helper()
	p := filepath.Join(dir, "config.json")
	if err := os.WriteFile(p, []byte(cfg), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("PI_SWITCH_CONFIG", p)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	return p
}

func postFetch(t *testing.T, r interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, url string) (int, map[string]interface{}) {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", url, nil)
	r.ServeHTTP(w, req)
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp
}

func loadCfg(t *testing.T, p string) config.PiSwitchConfig {
	t.Helper()
	cfg, _, err := config.LoadConfigAtPath(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return cfg
}

func TestChannelFetch_DirectedMergesIntoChannelPool(t *testing.T) {
	dir := t.TempDir()
	mockA := channelMock(t, "a1", "a2")
	defer mockA.Close()
	mockB := channelMock(t, "b1")
	defer mockB.Close()
	cfgJSON := `{"version":2,"profiles":{"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","models":[],"upstreams":[{"name":"main","baseUrl":"` + mockA.URL + `","apiKey":"ka"},{"name":"bk","baseUrl":"` + mockB.URL + `","apiKey":"kb"}]}},"settings":{"providerPrefix":"pi-switch"}}`
	p := writeChannelConfig(t, dir, cfgJSON)
	r := NewMgmtRouter()

	code, resp := postFetch(t, r, "/api/profiles/sup/fetch-models?channel=bk")
	if code != 200 {
		t.Fatalf("directed fetch code = %d, want 200 (%v)", code, resp)
	}
	if models, _ := resp["models"].([]interface{}); len(models) != 1 {
		t.Fatalf("resp models = %v, want [b1]", resp["models"])
	}
	cfg := loadCfg(t, p)
	prof := cfg.Profiles["sup"]
	mainModels, _ := prof.ChannelView("main")
	if len(mainModels) != 0 {
		t.Fatalf("main pool = %v, want empty (untouched)", mainModels)
	}
	bkModels, _ := prof.ChannelView("bk")
	if len(bkModels) != 1 || bkModels[0].ID != "b1" {
		t.Fatalf("bk pool = %v, want [b1]", bkModels)
	}
	if bkModels[0].ContextWindow != 128000 || bkModels[0].MaxTokens != 16384 {
		t.Fatalf("bk defaults wrong: %+v", bkModels[0])
	}
}

func TestChannelFetch_UnknownChannel400(t *testing.T) {
	dir := t.TempDir()
	mockA := channelMock(t, "a1")
	defer mockA.Close()
	cfgJSON := `{"version":2,"profiles":{"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","models":[],"upstreams":[{"name":"main","baseUrl":"` + mockA.URL + `","apiKey":"ka"}]}},"settings":{"providerPrefix":"pi-switch"}}`
	writeChannelConfig(t, dir, cfgJSON)
	r := NewMgmtRouter()
	if code, _ := postFetch(t, r, "/api/profiles/sup/fetch-models?channel=nope"); code != 400 {
		t.Fatalf("unknown channel code = %d, want 400", code)
	}
	if code, _ := postFetch(t, r, "/api/profiles/none/fetch-models?channel=main"); code != 404 {
		t.Fatalf("unknown profile code = %d, want 404", code)
	}
}

func TestChannelFetch_LegacyNoParamUnchanged(t *testing.T) {
	dir := t.TempDir()
	mockA := channelMock(t, "a1")
	defer mockA.Close()
	cfgJSON := `{"version":2,"profiles":{"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"` + mockA.URL + `","apiKey":"k","models":[{"id":"old","contextWindow":128000,"maxTokens":16384}]}},"settings":{"providerPrefix":"pi-switch"}}`
	p := writeChannelConfig(t, dir, cfgJSON)
	before, _ := os.ReadFile(p)
	r := NewMgmtRouter()
	code, resp := postFetch(t, r, "/api/profiles/sup/fetch-models")
	if code != 200 {
		t.Fatalf("legacy fetch code = %d, want 200", code)
	}
	if models, _ := resp["models"].([]interface{}); len(models) != 1 {
		t.Fatalf("resp models = %v, want [a1]", resp["models"])
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatalf("legacy no-param fetch must not rewrite config file")
	}
}

func TestChannelFetch_FailureDoesNotTouchOtherChannel(t *testing.T) {
	dir := t.TempDir()
	failMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"down"}`))
	}))
	defer failMock.Close()
	cfgJSON := `{"version":2,"profiles":{"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","models":[],"upstreams":[{"name":"bad","baseUrl":"` + failMock.URL + `","apiKey":"k"}]}},"settings":{"providerPrefix":"pi-switch"}}`
	p := writeChannelConfig(t, dir, cfgJSON)
	r := NewMgmtRouter()
	if code, _ := postFetch(t, r, "/api/profiles/sup/fetch-models?channel=bad"); code != 500 {
		t.Fatalf("failing channel code = %d, want 500", code)
	}
	cfg := loadCfg(t, p)
	if models, _ := cfg.Profiles["sup"].ChannelView("bad"); len(models) != 0 {
		t.Fatalf("failed channel pool = %v, want empty", models)
	}
}
