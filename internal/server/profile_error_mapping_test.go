package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Review finding (Standards axis, hard): the profile handlers classified errors
// by matching message text — `strings.Contains(err.Error(), "not found")` — even
// though this repo already established the alternative in the package domain
// (package_handlers.go's ErrPackageNotFound, used via errors.Is by both the HTTP
// handlers and the CLI).
//
// These tests pin the two consequences that motive the change:
//   1. a persist failure whose PATH happens to contain "not found" was answered
//      404 (the not-found arm was matched first) instead of 500;
//   2. the duplicate handler's 400 body had drifted from "as required" to the
//      CLI's own flag syntax, "--as <new> required".

// notFoundNamedUnwritableConfig blocks the config write while the config path
// itself contains the text "not found". The odd directory name IS the fixture:
// the persistence error embeds the temp file path, so this reproduces exactly the
// input that a message-text classifier cannot survive.
//
// Same injection strategy and the same honest coupling note as
// save_failure_test.go's unwritableConfig: the blocked write is the temp-file
// creation, so a refactor to an in-place write would stop injecting failure — the
// assertions below fail loudly if the write ever starts succeeding.
func notFoundNamedUnwritableConfig(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root: directory permissions do not restrict writes")
	}
	dir := filepath.Join(t.TempDir(), "config not found")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(configJSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })
	if !strings.Contains(cfgPath, "not found") {
		t.Fatalf("fixture broken: path %q must contain the misleading text", cfgPath)
	}

	isolateConfig(t)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	return cfgPath
}

// C1: a storage failure is 500 even when its message text matches the
// "not found" the old classifier looked for.
func TestProfileErrorMapping_PersistFailureIsNotMisreadAsNotFound(t *testing.T) {
	cfgPath := notFoundNamedUnwritableConfig(t)

	// Precondition: the write really does fail through this path.
	if err := os.WriteFile(filepath.Join(filepath.Dir(cfgPath), "probe"), nil, 0644); err == nil {
		t.Fatal("fixture broken: the directory is writable, so no failure is injected")
	}

	r := NewMgmtRouter()
	w := callMgmt(r, http.MethodPost, "/api/profiles/p1/duplicate", `{"as":"p2"}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("duplicate with a blocked config write = %d (%s), want 500", w.Code, w.Body.String())
	}
}

// C2: the duplicate endpoint keeps its own vocabulary — a missing target is a
// rejected request (400 "as required"), not a 404 and not the CLI's flag syntax.
func TestDuplicateProfile_MissingTargetKeepsServerWording(t *testing.T) {
	writableConfig(t)
	r := NewMgmtRouter()

	w := callMgmt(r, http.MethodPost, "/api/profiles/p1/duplicate", `{"as":""}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("duplicate without a target = %d (%s), want 400", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (%s)", err, w.Body.String())
	}
	if body["error"] != "as required" {
		t.Fatalf(`error = %q, want "as required" (the body before the CLI batch shipped CLI flag syntax)`, body["error"])
	}
}

// C3: the classification the endpoints already promised must not change: an
// unknown profile is 404, an unknown channel is a 400 naming the channel, and a
// duplicate target is 400.
func TestProfileErrorMapping_KeepsExistingStatuses(t *testing.T) {
	writableConfig(t)
	r := NewMgmtRouter()

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
		substr string
	}{
		{"duplicate unknown profile", http.MethodPost, "/api/profiles/nope/duplicate", `{"as":"p2"}`, 404, "not found"},
		{"duplicate existing target", http.MethodPost, "/api/profiles/p1/duplicate", `{"as":"p1"}`, 400, "target exists"},
		{"expose unknown profile", http.MethodPut, "/api/profiles/nope/expose?channel=main", `{"modelIds":["m1"]}`, 404, "not found"},
		{"expose unknown channel", http.MethodPut, "/api/profiles/p1/expose?channel=nope", `{"modelIds":["m1"]}`, 400, `unknown channel "nope"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := callMgmt(r, tc.method, tc.path, tc.body)
			if w.Code != tc.want {
				t.Fatalf("%s = %d (%s), want %d", tc.name, w.Code, w.Body.String(), tc.want)
			}
			var body map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v (%s)", err, w.Body.String())
			}
			if !strings.Contains(body["error"], tc.substr) {
				t.Fatalf("%s error = %q, want it to contain %q", tc.name, body["error"], tc.substr)
			}
		})
	}
}
