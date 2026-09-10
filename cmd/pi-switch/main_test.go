package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/server"
)

// The bind address reaches the auth guard through flag parsing, so the flag
// path is where a fail-open would hide: `--host ""` must arrive as an empty
// host (which the guard refuses), not be silently replaced by a local default.

func TestParseHostPort_FlagPath(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantHost string
		wantPort int
		wantDae  bool
	}{
		{"defaults when no flags", nil, "127.0.0.1", 43110, false},
		{"explicit wildcard", []string{"--host", "0.0.0.0"}, "0.0.0.0", 43110, false},
		{"explicit loopback", []string{"--host", "127.0.0.1"}, "127.0.0.1", 43110, false},
		{"empty host is passed through, not defaulted", []string{"--host", ""}, "", 43110, false},
		{"port and daemon", []string{"--host", "0.0.0.0", "--port", "43999", "--daemon"}, "0.0.0.0", 43999, true},
		{"invalid port keeps default", []string{"--port", "not-a-number"}, "127.0.0.1", 43110, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, port, daemon := parseHostPort(tc.args, "127.0.0.1", 43110)
			if host != tc.wantHost || port != tc.wantPort || daemon != tc.wantDae {
				t.Fatalf("parseHostPort(%q) = (%q, %d, %v), want (%q, %d, %v)",
					tc.args, host, port, daemon, tc.wantHost, tc.wantPort, tc.wantDae)
			}
		})
	}
}

func TestHasGeneratePasswordFlag(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"webui", "start", "--host", "0.0.0.0"}, false},
		{[]string{"webui", "start", "--host", "0.0.0.0", "--generate-password"}, true},
		{[]string{"--generate-password"}, true},
	}
	for _, tc := range cases {
		if got := hasGeneratePasswordFlag(tc.args); got != tc.want {
			t.Fatalf("hasGeneratePasswordFlag(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

// isolateCLI points every stateful path at a temp dir so these tests never touch
// the developer's real config or database.
func isolateCLI(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG", filepath.Join(dir, "config.json"))
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	t.Setenv("PI_AGENT_ROOT", filepath.Join(dir, "agent"))
	// piagent reads these ahead of PI_AGENT_ROOT, so `package import` would
	// otherwise consult whatever the developer exported.
	t.Setenv("PI_AGENT_SETTINGS", filepath.Join(dir, "agent", "settings.json"))
	t.Setenv("PI_CODING_AGENT_DIR", "")
	t.Setenv("PI_SWITCH_WEBUI_PASSWORD_FILE", filepath.Join(dir, "webui_password"))
	t.Setenv("PI_SWITCH_WEBUI_PASSWORD", "")
	// daemon state (pid/lock/log) would otherwise be read from the real
	// ~/.pi-switch and report the developer's running daemons.
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
}

// runCLI captures stdout while fn runs, so a test asserts what a script
// consuming this CLI would actually read.
func runCLI(t *testing.T, fn func() int) (int, string) {
	t.Helper()
	code, out, _ := runCLIStreams(t, fn)
	return code, out
}

// runCLIStreams also captures stderr, so failure paths can be checked for the
// user-visible reason instead of only for the absence of a success payload.
func runCLIStreams(t *testing.T, fn func() int) (int, string, string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = wOut, wErr
	code := fn()
	_ = wOut.Close()
	_ = wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	out, _ := io.ReadAll(rOut)
	errOut, _ := io.ReadAll(rErr)
	_ = rOut.Close()
	_ = rErr.Close()
	return code, string(out), string(errOut)
}

// B1: `package add` must write a real record that `package list` can see.
func TestHandlePackage_AddThenListShowsRealRecord(t *testing.T) {
	isolateCLI(t)

	code, out := runCLI(t, func() int { return handlePackage([]string{"add", "npm:demo-package"}) })
	if code != 0 {
		t.Fatalf("package add exit = %d, stdout=%q", code, out)
	}
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("package add stdout = %q, want ok payload", out)
	}

	code, out = runCLI(t, func() int { return handlePackage([]string{"list"}) })
	if code != 0 {
		t.Fatalf("package list exit = %d", code)
	}
	var body struct {
		Packages []map[string]any `json:"packages"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode list: %v (%q)", err, out)
	}
	if len(body.Packages) != 1 {
		t.Fatalf("listed %d packages, want 1: %q", len(body.Packages), out)
	}
	if got := body.Packages[0]["id"]; got != "npm:demo-package" {
		t.Fatalf("listed id = %v, want npm:demo-package", got)
	}
	if got := body.Packages[0]["installed"]; got != true {
		t.Fatalf("listed installed = %v, want true", got)
	}
}

// B2: `package show` reports a real record and fails for an unknown id.
func TestHandlePackage_ShowReflectsReality(t *testing.T) {
	isolateCLI(t)
	if code, out := runCLI(t, func() int { return handlePackage([]string{"add", "npm:shown"}) }); code != 0 {
		t.Fatalf("setup add failed: %d %q", code, out)
	}

	code, out := runCLI(t, func() int { return handlePackage([]string{"show", "npm:shown"}) })
	if code != 0 {
		t.Fatalf("show known exit = %d, stdout=%q", code, out)
	}
	var shown map[string]any
	if err := json.Unmarshal([]byte(out), &shown); err != nil {
		t.Fatalf("show stdout is not a JSON object: %v (%q)", err, out)
	}
	if shown["id"] != "npm:shown" {
		t.Fatalf("show id = %v, want npm:shown", shown["id"])
	}

	code, out, errOut := runCLIStreams(t, func() int { return handlePackage([]string{"show", "npm:absent"}) })
	if code == 0 {
		t.Fatalf("show unknown exit = 0, want non-zero (stdout=%q)", out)
	}
	if strings.Contains(out, `"ok"`) {
		t.Fatalf("show unknown printed ok: %q", out)
	}
	if !strings.Contains(errOut, "not found") {
		t.Fatalf("show unknown did not explain itself on stderr: %q", errOut)
	}
}

// B3: `package delete` removes a real record, and a second delete reports the
// truth instead of printing ok again.
func TestHandlePackage_DeleteRemovesThenReportsMissing(t *testing.T) {
	isolateCLI(t)
	if code, out := runCLI(t, func() int { return handlePackage([]string{"add", "npm:doomed"}) }); code != 0 {
		t.Fatalf("setup add failed: %d %q", code, out)
	}

	code, out := runCLI(t, func() int { return handlePackage([]string{"delete", "npm:doomed"}) })
	if code != 0 {
		t.Fatalf("delete exit = %d, stdout=%q", code, out)
	}

	_, out = runCLI(t, func() int { return handlePackage([]string{"list"}) })
	var body struct {
		Packages []map[string]any `json:"packages"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(body.Packages) != 0 {
		t.Fatalf("deleted package still listed: %q", out)
	}

	code, out = runCLI(t, func() int { return handlePackage([]string{"delete", "npm:doomed"}) })
	if code == 0 {
		t.Fatalf("deleting a missing package exit = 0, want non-zero (stdout=%q)", out)
	}
	if strings.Contains(out, `"ok"`) {
		t.Fatalf("deleting a missing package printed ok: %q", out)
	}
}

// B4: `package add` without a spec must fail rather than print a fake id.
func TestHandlePackage_AddRequiresSpec(t *testing.T) {
	isolateCLI(t)

	code, out, errOut := runCLIStreams(t, func() int { return handlePackage([]string{"add"}) })

	if code == 0 {
		t.Fatalf("package add with no spec exit = 0, want non-zero")
	}
	if strings.Contains(out, `"ok"`) {
		t.Fatalf("package add with no spec printed ok: %q", out)
	}
	if !strings.Contains(errOut, "spec") {
		t.Fatalf("package add with no spec did not explain itself on stderr: %q", errOut)
	}

	// Extra positionals must be refused rather than silently dropped.
	code, out, errOut = runCLIStreams(t, func() int {
		return handlePackage([]string{"add", "npm:x", "Name", "1.2.3"})
	})
	if code == 0 {
		t.Fatalf("package add with extra arguments exit = 0, want non-zero")
	}
	if strings.Contains(out, `"ok"`) {
		t.Fatalf("package add with extra arguments printed ok: %q", out)
	}
	if !strings.Contains(errOut, "one spec") {
		t.Fatalf("package add with extra arguments did not explain itself: %q", errOut)
	}
}

// B5: unimplemented subcommands report failure and never claim success.
func TestHandlePackage_UnimplementedCommandsFail(t *testing.T) {
	isolateCLI(t)

	for _, args := range [][]string{{"sync"}, {"nonsense"}} {
		code, out, errOut := runCLIStreams(t, func() int { return handlePackage(args) })
		if code == 0 {
			t.Fatalf("package %v exit = 0, want non-zero", args)
		}
		if strings.Contains(out, `"ok"`) {
			t.Fatalf("package %v printed ok: %q", args, out)
		}
		if strings.TrimSpace(errOut) == "" {
			t.Fatalf("package %v failed without saying why on stderr", args)
		}
	}
}

// B6: `ccs import` claims nothing because it does nothing; `ccs list` keeps its
// honest empty result.
func TestHandleCcs_ImportIsNotFakeSuccess(t *testing.T) {
	isolateCLI(t)

	code, out, errOut := runCLIStreams(t, func() int { return handleCcs([]string{"import"}) })
	if code == 0 {
		t.Fatalf("ccs import exit = 0, want non-zero")
	}
	if !strings.Contains(errOut, "not implemented") {
		t.Fatalf("ccs import did not say it is unimplemented: %q", errOut)
	}
	if strings.Contains(out, `"ok"`) || strings.Contains(out, "imported") {
		t.Fatalf("ccs import claimed a result: %q", out)
	}

	code, out = runCLI(t, func() int { return handleCcs([]string{"list"}) })
	if code != 0 {
		t.Fatalf("ccs list exit = %d, want 0 (an empty list is the truth)", code)
	}
	if !strings.Contains(out, `"providers":[]`) {
		t.Fatalf("ccs list stdout = %q, want an empty providers list", out)
	}
}

// B7: `stats` has no CLI implementation, so it must say so and fail instead of
// printing a hint that reads like a result.
func TestHandleStatsCLI_ReportsUnimplemented(t *testing.T) {
	isolateCLI(t)

	code, out, errOut := runCLIStreams(t, handleStatsCLI)

	if code == 0 {
		t.Fatalf("stats exit = 0, want non-zero while unimplemented")
	}
	if !strings.Contains(errOut, "not implemented") {
		t.Fatalf("stats did not say it is unimplemented: %q", errOut)
	}
	if strings.Contains(out, `"ok"`) {
		t.Fatalf("stats printed ok: %q", out)
	}
}

// B8: `preset` must list the real preset catalogue, not an empty array, and must
// fail for an unknown id.
func TestHandlePresets_ListsRealCatalogue(t *testing.T) {
	isolateCLI(t)

	code, out := runCLI(t, func() int { return handlePresets(nil) })
	if code != 0 {
		t.Fatalf("preset exit = %d", code)
	}
	var presets []map[string]any
	if err := json.Unmarshal([]byte(out), &presets); err != nil {
		t.Fatalf("decode presets: %v (%q)", err, out)
	}
	if len(presets) == 0 {
		t.Fatalf("preset printed an empty catalogue: %q", out)
	}
	names := map[string]bool{}
	for _, p := range presets {
		if id, ok := p["id"].(string); ok {
			names[id] = true
		}
	}
	if !names["openai"] {
		t.Fatalf("preset catalogue is missing openai: %q", out)
	}

	code, out = runCLI(t, func() int { return handlePresets([]string{"show", "openai"}) })
	if code != 0 {
		t.Fatalf("preset show openai exit = %d, stdout=%q", code, out)
	}
	var preset map[string]any
	if err := json.Unmarshal([]byte(out), &preset); err != nil {
		t.Fatalf("preset show stdout is not a JSON object: %v (%q)", err, out)
	}
	if preset["id"] != "openai" {
		t.Fatalf("preset id = %v, want openai", preset["id"])
	}

	code, out = runCLI(t, func() int { return handlePresets([]string{"show", "nope"}) })
	if code == 0 {
		t.Fatalf("preset show unknown exit = 0, want non-zero (stdout=%q)", out)
	}

	// The documented `presets list` spelling must work, not fall through to the
	// unknown-id path.
	code, out = runCLI(t, func() int { return handlePresets([]string{"list"}) })
	if code != 0 {
		t.Fatalf("preset list exit = %d, want 0 (stdout=%q)", code, out)
	}
	if !strings.Contains(out, "openai") {
		t.Fatalf("preset list stdout = %q, want the catalogue", out)
	}
}

// B11: `config validate` must not call a corrupt file valid. The loader falls
// back to a default config containing a placeholder profile, so the old check
// ("has profiles") always reported success.
func TestHandleConfigCLI_ValidateRejectsCorruptFile(t *testing.T) {
	isolateCLI(t)
	if err := os.WriteFile(os.Getenv("PI_SWITCH_CONFIG"), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := runCLIStreams(t, func() int { return handleConfigCLI([]string{"validate"}) })

	if code == 0 {
		t.Fatalf("config validate exit = 0 for a corrupt file, want non-zero (stdout=%q)", out)
	}
	if strings.Contains(out, "config valid") {
		t.Fatalf("config validate called a corrupt file valid: %q", out)
	}
	if !strings.Contains(errOut, "invalid") {
		t.Fatalf("config validate did not explain the failure: %q", errOut)
	}
}

// B9: `doctor` reports real probes and succeeds on a healthy temp setup.
func TestHandleDoctor_ReportsRealProbes(t *testing.T) {
	isolateCLI(t)

	code, out := runCLI(t, handleDoctor)

	if code != 0 {
		t.Fatalf("doctor exit = %d on a healthy setup, want 0 (stdout=%q)", code, out)
	}
	for _, want := range []string{"config:", "database:", "proxy daemon:", "webui daemon:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output %q is missing %q", out, want)
		}
	}
}

// B9b: with a database present, a failing probe must still keep stdout free of
// any success token — the database line used to print "ok".
func TestHandleDoctor_NoSuccessTokenWhenDatabaseExists(t *testing.T) {
	isolateCLI(t)
	if code, out := runCLI(t, func() int { return handlePackage([]string{"add", "npm:seed"}) }); code != 0 {
		t.Fatalf("setup add failed: %d %q", code, out)
	}
	if _, err := os.Stat(server.PiSwitchDBPath()); err != nil {
		t.Fatalf("setup did not create a database: %v", err)
	}
	if err := os.WriteFile(os.Getenv("PI_SWITCH_CONFIG"), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}

	code, out := runCLI(t, handleDoctor)

	if code == 0 {
		t.Fatalf("doctor exit = 0 with a corrupt config, want non-zero")
	}
	// Assert on success tokens, not on the substring "ok" (which occurs inside
	// "not running").
	for _, token := range []string{"doctor: ok", "database: ok", "readable ok"} {
		if strings.Contains(out, token) {
			t.Fatalf("doctor stdout contains success token %q while failing: %q", token, out)
		}
	}
}

// B10: `doctor` must not claim ok when the config is unreadable.
func TestHandleDoctor_FailsOnUnreadableConfig(t *testing.T) {
	isolateCLI(t)
	cfgPath := os.Getenv("PI_SWITCH_CONFIG")
	if err := os.WriteFile(cfgPath, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}

	code, out := runCLI(t, handleDoctor)

	if code == 0 {
		t.Fatalf("doctor exit = 0 with a corrupt config, want non-zero (stdout=%q)", out)
	}
	if strings.Contains(out, "doctor: ok") {
		t.Fatalf("doctor claimed ok with a corrupt config: %q", out)
	}
	if !strings.Contains(out, "unreadable") {
		t.Fatalf("doctor did not explain the failure: %q", out)
	}
}

// unreadableWriteConfig keeps the config readable while making every write fail,
// so `provider use/delete` reach their save call and must report the failure.
func unreadableWriteConfigCLI(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root: directory permissions do not restrict writes")
	}
	dir := t.TempDir()
	dbDir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	body := `{"version":2,"profiles":{"p1":{"api":"openai-responses","responsesMode":"passthrough","baseUrl":"http://x","apiKey":"k","models":[{"id":"m1","contextWindow":100,"maxTokens":10}]}},"settings":{}}`
	if err := os.WriteFile(cfgPath, []byte(body), 0644); err != nil {
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

// A10: a provider change that cannot be persisted must not report success.
func TestHandleProvider_ReportsSaveFailure(t *testing.T) {
	unreadableWriteConfigCLI(t)

	for _, args := range [][]string{{"use", "p1"}, {"delete", "p1"}} {
		code, out, errOut := runCLIStreams(t, func() int { return handleProvider(args) })
		if code == 0 {
			t.Fatalf("provider %v exit = 0 while the config write fails (stdout=%q)", args, out)
		}
		if strings.Contains(out, "Switched to") || strings.Contains(out, "Deleted ") {
			t.Fatalf("provider %v still reported success: %q", args, out)
		}
		if !strings.Contains(errOut, "failed to save config") {
			t.Fatalf("provider %v did not explain the save failure: %q", args, errOut)
		}
	}
}

// Positive control: with a writable config the same commands succeed.
func TestHandleProvider_SucceedsOnWritableConfig(t *testing.T) {
	isolateCLI(t)
	cfgPath := os.Getenv("PI_SWITCH_CONFIG")
	body := `{"version":2,"profiles":{"p1":{"api":"openai-responses","responsesMode":"passthrough","baseUrl":"http://x","apiKey":"k","models":[]}},"settings":{}}`
	if err := os.WriteFile(cfgPath, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	code, out := runCLI(t, func() int { return handleProvider([]string{"use", "p1"}) })

	if code != 0 {
		t.Fatalf("provider use exit = %d, want 0 (stdout=%q)", code, out)
	}
	if !strings.Contains(out, "Switched to p1") {
		t.Fatalf("provider use stdout = %q, want the switch confirmation", out)
	}
}
