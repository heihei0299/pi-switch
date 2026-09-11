package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/protocol"
)

// ARCH-01: a corrupt config must fail the read endpoints explicitly instead of
// being served as a default/empty config.
func TestStrictConfig_CorruptFileFailsReadEndpoints(t *testing.T) {
	cfgPath := isolateConfig(t)
	if err := os.WriteFile(cfgPath, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	r := NewMgmtRouter()
	for _, path := range []string{"/api/config", "/api/state", "/api/profiles", "/api/settings"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("GET %s with corrupt config = %d, want 500 (body=%s)", path, w.Code, w.Body.String())
		}
		// system-contract 2.8: management errors are a bare string message.
		var body struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error == "" {
			t.Fatalf("GET %s management envelope = %s, want {\"error\":\"...\"}", path, w.Body.String())
		}
	}
}

// system-contract 2.8: the inference surface keeps the OpenAI error object even
// when the failure is a shared pre-step such as reading the config.
func TestStrictConfig_InferenceErrorEnvelope(t *testing.T) {
	cfgPath := isolateConfig(t)
	if err := os.WriteFile(cfgPath, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	NewProxyRouter().ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("GET /v1/models with corrupt config = %d, want 500 (body=%s)", w.Code, w.Body.String())
	}
	var body struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode /v1/models error: %v (%s)", err, w.Body.String())
	}
	if body.Error.Message == "" || body.Error.Type == "" {
		t.Fatalf("GET /v1/models inference envelope = %s, want {\"error\":{\"message\":...,\"type\":...}}", w.Body.String())
	}
}

// A missing file is still the default config, so the read endpoints keep working
// before the first save.
func TestStrictConfig_MissingFileStillServesDefault(t *testing.T) {
	isolateConfig(t) // config file intentionally not created
	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/settings without config = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
}

// system-contract §2.2「写入门的校验强度差异是有意的」：整文件门保存的是 loader
// 能加载、进程能运行的东西，所以它不得比 loader 更严；同一个 profile 走 Profile CRUD
// 则必须被拒。这条测试把两侧一起钉住，防止有人"顺手统一"后把不对称换成
// 「能被自己加载运行、却拒绝保存」。
func TestPutConfig_WholeFileDoorStaysTolerant(t *testing.T) {
	isolateConfig(t)
	r := NewMgmtRouter()

	// 形状不合格（upstream baseUrl 没有 scheme）：CRUD 会拒，整文件门必须收下。
	prof := `{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://example.test/v1","apiKey":"k","upstreams":[{"name":"main","api":"openai-completions","baseUrl":"ftp://example.test","apiKey":"k","models":[{"id":"m1"}]}]}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{"version":2,"profiles":{"p":`+prof+`}}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/config with an ftp upstream = %d, want 200 (the whole-file door must not be stricter than the loader): %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/profiles", strings.NewReader(`{"name":"p","profile":`+prof+`}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/profiles with the same profile = %d, want 400 (Profile CRUD stays strict): %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "baseUrl") {
		t.Fatalf("CRUD rejection should name the offending field: %s", w.Body.String())
	}
}

// ARCH-02: PUT /api/config parses the body into the typed config before it
// persists, so a body the strict loader would reject can never be written, and
// the save goes through the 0600 atomic writer.
func TestPutConfig_ParsesBeforePersist(t *testing.T) {
	cfgPath := isolateConfig(t)
	r := NewMgmtRouter()
	put := func(body string) int {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w.Code
	}

	if code := put(`{"version":"two"}`); code != http.StatusBadRequest {
		t.Fatalf("PUT config with a wrong field type = %d, want 400", code)
	}
	if code := put(`{"version":2,"profiles":{"p":{"api":"openai-completions","responsesMode":"passthrough"}}}`); code != http.StatusBadRequest {
		t.Fatalf("PUT config with an invalid responsesMode = %d, want 400", code)
	}
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Fatalf("a rejected PUT must not create %s (stat err=%v)", cfgPath, err)
	}

	body := `{"version":2,"current":"p","profiles":{"p":{"api":"openai-completions","baseUrl":"https://example.test/v1","apiKey":"k"}},"settings":{"writeMode":"gateway"}}`
	if code := put(body); code != http.StatusOK {
		t.Fatalf("PUT valid config = %d, want 200", code)
	}
	info, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if mode := info.Mode().Perm(); mode != 0600 {
			t.Fatalf("saved config mode = %o, want 600", mode)
		}
	}
	reloaded, _, err := config.LoadConfigAtPath(cfgPath)
	if err != nil {
		t.Fatalf("strict reload of PUT config: %v", err)
	}
	if _, ok := reloaded.Profiles["p"]; !ok {
		t.Fatalf("saved config lost profile p: %+v", reloaded.Profiles)
	}
	if reloaded.Settings.Proxy.Host != "127.0.0.1" || reloaded.Settings.Proxy.Port != 43112 {
		t.Fatalf("settings defaults not backfilled: %+v", reloaded.Settings.Proxy)
	}
}

// The WebUI takes its api list and responsesMode rule from /api/state, so the
// state payload must carry the protocol capability set.
func TestStateExposesProtocolCapabilities(t *testing.T) {
	isolateConfig(t)
	w := httptest.NewRecorder()
	NewMgmtRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/state", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/state = %d (%s), want 200", w.Code, w.Body.String())
	}
	var body struct {
		Protocol struct {
			APIs []struct {
				ID             string   `json:"id"`
				DefaultMode    string   `json:"defaultMode"`
				ResponsesModes []string `json:"responsesModes"`
			} `json:"apis"`
		} `json:"protocol"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode state: %v (%s)", err, w.Body.String())
	}
	if len(body.Protocol.APIs) != len(protocol.Capabilities()) {
		t.Fatalf("state.protocol.apis len = %d, want %d (%s)", len(body.Protocol.APIs), len(protocol.Capabilities()), w.Body.String())
	}
	for _, api := range body.Protocol.APIs {
		if api.ID == "" || api.DefaultMode == "" || len(api.ResponsesModes) == 0 {
			t.Fatalf("state.protocol.apis entry is incomplete: %+v", api)
		}
	}
}
