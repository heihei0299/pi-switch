package main

import (
	"encoding/json"
	"testing"
)

// Shared fixtures for the `provider` CLI tests. They assert through the CLI's own
// output (`provider show`) rather than by decoding config.json: a change the CLI
// cannot show is not a change the user made, and these tests should not break
// because the file layout changed.

// deadUpstreamBase is an address nothing listens on. Tests that only need a
// profile to exist use it, so no mock server is created for a request that is
// never sent — a mock that is never contacted asserts nothing.
const deadUpstreamBase = "http://127.0.0.1:9/v1"

// createOneChannelProfile makes a profile with a single `main` channel through
// the CLI, carrying m1 and m2.
func createOneChannelProfile(t *testing.T, name string) {
	t.Helper()
	if code, _, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"add", name, "--base-url", deadUpstreamBase, "--models", "m1,m2"})
	}); code != 0 {
		t.Fatalf("setup add failed: %d %q", code, errOut)
	}
}

// showProfile returns `provider show`'s output for a profile verbatim.
func showProfile(t *testing.T, name string) string {
	t.Helper()
	code, shown, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"show", name})
	})
	if code != 0 {
		t.Fatalf("provider show %s failed: %d %q", name, code, errOut)
	}
	return shown
}

// shownChannelExposed returns the ids one named channel exposes, as the CLI
// reports them.
func shownChannelExposed(t *testing.T, name, channel string) []string {
	t.Helper()
	var prof struct {
		Upstreams []struct {
			Name          string   `json:"name"`
			ExposedModels []string `json:"exposedModels"`
		} `json:"upstreams"`
	}
	if err := json.Unmarshal([]byte(showProfile(t, name)), &prof); err != nil {
		t.Fatalf("provider show %s did not print a profile: %v", name, err)
	}
	for _, u := range prof.Upstreams {
		if u.Name == channel {
			return u.ExposedModels
		}
	}
	t.Fatalf("profile %s has no channel %q", name, channel)
	return nil
}
