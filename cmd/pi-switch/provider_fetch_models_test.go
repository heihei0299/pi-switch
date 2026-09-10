package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Ticket provider-cli 04: `provider fetch-models` must really list the upstream's
// models. The server handler implements it (read-only GET); the CLI answered
// "unknown provider subcommand" while the help text advertised it.
//
// These tests never touch a real provider: they point the profile at a local
// mock, because the command spends upstream quota when pointed at a real one.

func mockModelsServer(t *testing.T, status int, ids ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("nope"))
			return
		}
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

// B1: the command prints the upstream's model ids.
func TestHandleProvider_FetchModelsListsUpstreamModels(t *testing.T) {
	isolateCLI(t)
	srv := mockModelsServer(t, http.StatusOK, "alpha", "beta")

	if code, _, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"add", "mock", "--base-url", srv.URL + "/v1"})
	}); code != 0 {
		t.Fatalf("setup add failed: %d %q", code, errOut)
	}

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"fetch-models", "mock"})
	})
	if code != 0 {
		t.Fatalf("provider fetch-models exit = %d (stdout=%q stderr=%q)", code, out, errOut)
	}
	for _, id := range []string{"alpha", "beta"} {
		if !strings.Contains(out, id) {
			t.Fatalf("model %q missing from output: %q", id, out)
		}
	}
}

// B2: an upstream that refuses is reported as a failure, not as an empty list,
// and nothing is written to the config.
func TestHandleProvider_FetchModelsReportsUpstreamFailure(t *testing.T) {
	isolateCLI(t)
	cfgPath := os.Getenv("PI_SWITCH_CONFIG")
	srv := mockModelsServer(t, http.StatusUnauthorized)

	if code, _, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"add", "mock", "--base-url", srv.URL + "/v1"})
	}); code != 0 {
		t.Fatalf("setup add failed: %d %q", code, errOut)
	}
	before, _ := os.ReadFile(cfgPath)

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"fetch-models", "mock"})
	})

	if code == 0 {
		t.Fatalf("fetching from a failing upstream exit = 0 (stdout=%q)", out)
	}
	if !strings.Contains(errOut, "401") {
		t.Fatalf("the upstream status was not reported: %q", errOut)
	}
	after, _ := os.ReadFile(cfgPath)
	if string(before) != string(after) {
		t.Fatal("a failed fetch modified the config")
	}
}

// B3: an unknown profile is refused before any network attempt.
func TestHandleProvider_FetchModelsRejectsUnknownProfile(t *testing.T) {
	isolateCLI(t)

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"fetch-models", "nope"})
	})

	if code == 0 {
		t.Fatalf("fetching for an unknown profile exit = 0 (stdout=%q)", out)
	}
	if !strings.Contains(errOut, "unknown profile") {
		t.Fatalf("unknown profile was not reported: %q", errOut)
	}
}
