package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Ticket provider-cli 05: `provider expose` was documented in both READMEs but
// answered "unknown provider subcommand". The server implements it
// (PUT /api/profiles/:name/expose); the CLI is now wired to the same core.

func mockModelListServer(t *testing.T, ids ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := make([]map[string]string, 0, len(ids))
		for _, id := range ids {
			data = append(data, map[string]string{"id": id})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// setupSingleChannelProfile creates a one-channel profile carrying `ids` as
// models, by fetching them from a mock upstream (the honest path the CLI offers).
func setupSingleChannelProfile(t *testing.T, name string, ids ...string) string {
	t.Helper()
	isolateCLI(t)
	cfgPath := os.Getenv("PI_SWITCH_CONFIG")
	srv := mockModelListServer(t, ids...)
	_ = srv
	if code, _, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"add", name, "--base-url", srv.URL + "/v1", "--api", "openai-responses", "--models", strings.Join(ids, ",")})
	}); code != 0 {
		t.Fatalf("setup add failed: %d %q", code, errOut)
	}
	return cfgPath
}

// B1: exposing a model the channel carries is recorded in the config.
func TestHandleProvider_ExposeRecordsSelection(t *testing.T) {
	cfgPath := setupSingleChannelProfile(t, "exp", "m1", "m2")

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"expose", "exp", "m1"})
	})
	if code != 0 {
		t.Fatalf("provider expose exit = %d (stdout=%q stderr=%q)", code, out, errOut)
	}
	profiles := readProfiles(t, cfgPath)
	prof, _ := profiles["exp"].(map[string]any)
	upstreams, _ := prof["upstreams"].([]any)
	if len(upstreams) == 0 {
		t.Fatalf("profile has no upstreams: %v", prof)
	}
	first, _ := upstreams[0].(map[string]any)
	exposed, _ := first["exposedModels"].([]any)
	if len(exposed) != 1 || exposed[0] != "m1" {
		t.Fatalf("exposedModels = %v, want [m1]", first["exposedModels"])
	}
}

// B2: exposing an id the channel does not carry is refused — otherwise routing
// would advertise a model nothing can serve.
func TestHandleProvider_ExposeRefusesUnknownModel(t *testing.T) {
	setupSingleChannelProfile(t, "exp", "m1")

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"expose", "exp", "nope"})
	})

	if code == 0 {
		t.Fatalf("exposing an unknown model exit = 0 (stdout=%q)", out)
	}
	if !strings.Contains(errOut, "unknown model") {
		t.Fatalf("the reason was not reported: %q", errOut)
	}
}

// B3: an unknown profile is refused.
func TestHandleProvider_ExposeRejectsUnknownProfile(t *testing.T) {
	isolateCLI(t)

	code, _, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"expose", "nope", "m1"})
	})

	if code == 0 {
		t.Fatal("exposing on an unknown profile exit = 0")
	}
	// Same CLI wording as show/use/delete ("unknown profile"), not the server's.
	if !strings.Contains(errOut, "unknown profile") {
		t.Fatalf("unknown profile was not reported: %q", errOut)
	}
}
