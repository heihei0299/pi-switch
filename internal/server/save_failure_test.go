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

// A10: a failed config write must surface as a failure. These endpoints used to
// discard saveConfig's error and answer 200 {"ok":true}, so a full disk or a
// permission problem silently discarded the user's change.

// configJSON carries one profile with a named channel, so every write endpoint
// below reaches its save call.
const configJSON = `{"version":2,"profiles":{"p1":{"api":"openai-responses","responsesMode":"passthrough","baseUrl":"http://x","apiKey":"k","upstreams":[{"name":"main","api":"openai-responses","baseUrl":"http://x","apiKey":"k","models":[{"id":"m1","contextWindow":100,"maxTokens":10}],"exposedModels":["m1"]}]}},"settings":{}}`

// unwritableConfig injects a save failure the only way that keeps the config
// READABLE, which the profile endpoints require: a directory that permits reads
// but not the write that SaveAtPath performs.
//
// Coupling to state honestly: with the current temp-file+rename strategy the
// failure comes from creating config.json.tmp, so a refactor to an in-place write
// would no longer be injected by this fixture. Injectability is therefore
// ASSERTED below, and the B1 assertion (expect 500) fails loudly if the write
// ever starts succeeding — the fixture cannot rot into a silent pass.
func unwritableConfig(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root: directory permissions do not restrict writes")
	}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	isolateConfig(t)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)

	// Precondition: reads must still work (else the profile cases would 404 for
	// the wrong reason) and writes must fail (else this fixture injects nothing).
	if _, _, err := config.LoadConfigAtPath(cfgPath); err != nil {
		t.Fatalf("fixture broken: config must stay readable, got %v", err)
	}
	if err := config.SaveAtPath(config.DefaultConfig(), cfgPath); err == nil {
		t.Fatal("fixture broken: the config write unexpectedly succeeded, so no failure is injected")
	}
}

// writeFailureCases drives every endpoint that persists the config. The same
// table runs against a blocked config (expect 5xx) and a writable one (expect
// 200), so the contrast is attributable to writeability alone.
func writeFailureCases() []struct {
	name   string
	method string
	path   string
	body   string
} {
	return []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"put settings", http.MethodPut, "/api/settings", `{"providerPrefix":"pi-switch"}`},
		{"put config", http.MethodPut, "/api/config", configJSON},
		{"put models", http.MethodPut, "/api/profiles/p1/models", `{"models":[{"id":"m1","contextWindow":100,"maxTokens":10}],"channel":"main"}`},
		{"put expose", http.MethodPut, "/api/profiles/p1/expose?channel=main", `{"modelIds":["m1"]}`},
		{"duplicate profile", http.MethodPost, "/api/profiles/p1/duplicate", `{"as":"p2"}`},
		{"delete profile", http.MethodDelete, "/api/profiles/p1", ""},
	}
}

// writableConfig reuses the package's isolation norm and only adds a config file.
func writableConfig(t *testing.T) string {
	t.Helper()
	cfgPath := isolateConfig(t)
	if err := os.WriteFile(cfgPath, []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}
	return cfgPath
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
	if msg, _ := body["error"].(string); msg == "" {
		t.Fatalf("%s: error message is empty: %s", what, w.Body.String())
	}
}

// B1: every endpoint that writes the config reports the write failure instead of
// answering 200 with {"ok":true}.
func TestConfigWrites_ReportFailureInsteadOfSuccess(t *testing.T) {
	for _, tc := range writeFailureCases() {
		t.Run(tc.name, func(t *testing.T) {
			unwritableConfig(t)
			r := NewMgmtRouter()

			assertSaveFailure(t, callMgmt(r, tc.method, tc.path, tc.body), tc.name)
		})
	}
}

// B2: the positive control runs the SAME cases on a writable config, so each 500
// above is attributable to the blocked write rather than to the request.
func TestConfigWrites_SucceedOnWritableConfig(t *testing.T) {
	for _, tc := range writeFailureCases() {
		t.Run(tc.name, func(t *testing.T) {
			writableConfig(t)
			r := NewMgmtRouter()

			if w := callMgmt(r, tc.method, tc.path, tc.body); w.Code != http.StatusOK {
				t.Fatalf("%s on a writable config = %d (%s), want 200", tc.name, w.Code, w.Body.String())
			}
		})
	}
}

// B3: a refused save must leave the config untouched — the property the failure
// actually protects.
func TestConfigWrites_LeaveConfigIntactOnFailure(t *testing.T) {
	unwritableConfig(t)
	cfgPath := os.Getenv("PI_SWITCH_CONFIG")
	before, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	r := NewMgmtRouter()
	if w := callMgmt(r, http.MethodPut, "/api/settings", `{"providerPrefix":"changed"}`); w.Code != http.StatusInternalServerError {
		t.Fatalf("PUT /api/settings = %d, want 500", w.Code)
	}

	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config became unreadable after a failed save: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("config changed despite a refused save:\nbefore=%s\nafter=%s", before, after)
	}
}

// B4: the endpoint that writes config.json directly must not leave artifacts or
// a half-updated file when the save is refused.
//
// Coverage limit, stated honestly: the fixture blocks the TEMP WRITE, so this
// exercises the write-failure branch and proves no temp file survives it. The
// rename-failure branch that `handlePutConfig` used to swallow is NOT injected
// here — on Unix a rename failure is not reachable without a mock or a
// cross-device mount, so it has no automated regression test; the fix there was
// verified by reading (500 + os.Remove(tmp)) and by the reviewer reproducing the
// old behavior on a copy.
func TestPutConfig_LeavesNoTempFileBehindOnFailure(t *testing.T) {
	unwritableConfig(t)
	cfgPath := os.Getenv("PI_SWITCH_CONFIG")

	r := NewMgmtRouter()
	if w := callMgmt(r, http.MethodPut, "/api/config", configJSON); w.Code != http.StatusInternalServerError {
		t.Fatalf("PUT /api/config = %d, want 500", w.Code)
	}

	if _, err := os.Stat(cfgPath + ".tmp"); err == nil {
		t.Fatalf("a refused save left %s.tmp behind", cfgPath)
	}
	// The original config must be untouched as well.
	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config became unreadable: %v", err)
	}
	if string(after) != configJSON {
		t.Fatalf("config changed despite a refused save: %s", after)
	}
}
