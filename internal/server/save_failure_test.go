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

// Ticket A10: a failed config write must surface as a failure. These endpoints
// used to discard saveConfig's error and answer 200 {"ok":true}, so a full disk
// or a permission problem silently discarded the user's change.

// unreadableWriteConfig keeps the config *readable* (so request handling gets
// past routing and reaches the save) while making every write fail: the file
// lives in a directory that is not writable, so creating the temp file fails.
func unreadableWriteConfig(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root: directory permissions do not restrict writes")
	}
	dir := t.TempDir()
	dbDir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(validConfigJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dbDir, "requests.db"))
	t.Setenv("PI_AGENT_SESSIONS", filepath.Join(dbDir, "sessions"))
	t.Setenv("PI_SWITCH_WEBUI_PASSWORD_FILE", filepath.Join(dbDir, "webui_password"))
	return cfgPath
}

// validConfigJSON carries one profile with a named channel, so every write
// endpoint below reaches its save call.
const validConfigJSON = `{"version":2,"profiles":{"p1":{"api":"openai-responses","responsesMode":"passthrough","baseUrl":"http://x","apiKey":"k","upstreams":[{"name":"main","api":"openai-responses","baseUrl":"http://x","apiKey":"k","models":[{"id":"m1","contextWindow":100,"maxTokens":10}],"exposedModels":["m1"]}]}},"settings":{}}`

// writableConfig is the positive control: the same endpoint on a writable path
// answers 200, so a 500 above is attributable to the write failure.
func writableConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(validConfigJSON), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	t.Setenv("PI_AGENT_SESSIONS", filepath.Join(dir, "sessions"))
	t.Setenv("PI_SWITCH_WEBUI_PASSWORD_FILE", filepath.Join(dir, "webui_password"))
	return cfgPath
}

func callJSON(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func assertSaveFailure(t *testing.T, w *httptest.ResponseRecorder, what string) {
	t.Helper()
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("%s = %d (%s), want 500", what, w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s: decode body: %v (%s)", what, err, w.Body.String())
	}
	if _, claimed := body["ok"]; claimed {
		t.Fatalf("%s still claims success: %s", what, w.Body.String())
	}
	msg, _ := body["error"].(string)
	if msg == "" {
		t.Fatalf("%s: error message is empty: %s", what, w.Body.String())
	}
}

// B1: every endpoint that writes the config reports the write failure instead of
// answering 200 with {"ok":true}.
func TestConfigWrites_ReportFailureInsteadOfSuccess(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"put settings", http.MethodPut, "/api/settings", `{"providerPrefix":"pi-switch"}`},
		{"put models", http.MethodPut, "/api/profiles/p1/models", `{"models":[{"id":"m1","contextWindow":100,"maxTokens":10}],"channel":"main"}`},
		{"put expose", http.MethodPut, "/api/profiles/p1/expose?channel=main", `{"modelIds":["m1"]}`},
		{"duplicate profile", http.MethodPost, "/api/profiles/p1/duplicate", `{"as":"p2"}`},
		{"delete profile", http.MethodDelete, "/api/profiles/p1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			unreadableWriteConfig(t)
			r := NewMgmtRouter()

			assertSaveFailure(t, callJSON(r, tc.method, tc.path, tc.body), tc.name)
		})
	}
}

// B2: positive control — with a writable config the same endpoints still answer
// 200, so the 500s above come from the write failure and not from the request.
func TestConfigWrites_SucceedOnWritableConfig(t *testing.T) {
	writableConfig(t)
	r := NewMgmtRouter()

	if w := callJSON(r, http.MethodPut, "/api/settings", `{"providerPrefix":"pi-switch"}`); w.Code != http.StatusOK {
		t.Fatalf("PUT /api/settings = %d (%s), want 200", w.Code, w.Body.String())
	}
	if w := callJSON(r, http.MethodPut, "/api/profiles/p1/models", `{"models":[{"id":"m1","contextWindow":100,"maxTokens":10}],"channel":"main"}`); w.Code != http.StatusOK {
		t.Fatalf("PUT /api/profiles/p1/models = %d (%s), want 200", w.Code, w.Body.String())
	}
}
