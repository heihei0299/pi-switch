package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Ticket provider-cli 01: `provider add` must create the profile for real. It
// used to answer "unknown provider subcommand", even though the help text
// advertised it and POST /api/profiles implements it.

func readProfiles(t *testing.T, cfgPath string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var doc struct {
		Profiles map[string]any `json:"profiles"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("decode config: %v (%s)", err, b)
	}
	return doc.Profiles
}

// B1: add writes a profile that a later `provider list` shows.
func TestHandleProvider_AddCreatesProfile(t *testing.T) {
	isolateCLI(t)
	cfgPath := os.Getenv("PI_SWITCH_CONFIG")

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"add", "cli-made", "--base-url", "http://127.0.0.1:9/v1", "--api-key", "k"})
	})
	if code != 0 {
		t.Fatalf("provider add exit = %d (stdout=%q stderr=%q)", code, out, errOut)
	}

	profiles := readProfiles(t, cfgPath)
	if _, ok := profiles["cli-made"]; !ok {
		t.Fatalf("profile was not created; config has %v", profiles)
	}

	// Observable through the CLI itself, not only through the file.
	code, out = runCLI(t, func() int { return handleProvider([]string{"list"}) })
	if code != 0 {
		t.Fatalf("provider list exit = %d", code)
	}
	if !strings.Contains(out, "cli-made") {
		t.Fatalf("provider list does not show the new profile: %q", out)
	}
}

// B2: an empty name is refused rather than creating an unnamed profile.
func TestHandleProvider_AddRequiresName(t *testing.T) {
	isolateCLI(t)

	code, out, errOut := runCLIStreams(t, func() int { return handleProvider([]string{"add"}) })

	if code == 0 {
		t.Fatalf("provider add without a name exit = 0 (stdout=%q)", out)
	}
	if strings.TrimSpace(errOut) == "" {
		t.Fatal("provider add without a name failed without explaining why")
	}
}

// B3: adding an existing profile is refused (the server handler rejects it too),
// so `add` cannot silently overwrite a working supplier.
func TestHandleProvider_AddRefusesExistingName(t *testing.T) {
	isolateCLI(t)
	cfgPath := os.Getenv("PI_SWITCH_CONFIG")

	if code, _, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"add", "dupe", "--base-url", "http://127.0.0.1:9/v1"})
	}); code != 0 {
		t.Fatalf("setup add failed: %d %q", code, errOut)
	}
	before, _ := os.ReadFile(cfgPath)

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"add", "dupe", "--base-url", "http://127.0.0.1:8/v1"})
	})

	if code == 0 {
		t.Fatalf("re-adding an existing profile exit = 0 (stdout=%q)", out)
	}
	if !strings.Contains(errOut, "exists") {
		t.Fatalf("re-adding did not explain the collision: %q", errOut)
	}
	after, _ := os.ReadFile(cfgPath)
	if string(before) != string(after) {
		t.Fatal("a refused add still modified the config")
	}
}

// B4: an invalid baseUrl is refused by the same validation the server uses.
func TestHandleProvider_AddValidatesBaseURL(t *testing.T) {
	isolateCLI(t)

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"add", "bad-url", "--base-url", "not-a-url"})
	})

	if code == 0 {
		t.Fatalf("provider add with an invalid baseUrl exit = 0 (stdout=%q)", out)
	}
	if !strings.Contains(errOut, "baseUrl") {
		t.Fatalf("invalid baseUrl was not reported: %q", errOut)
	}
}

// B5: without --base-url `add` writes a flat profile, so the profile's own api is
// the effective pair and the CLI door must refuse a known-but-unproxyable one
// instead of storing a supplier every request through it would fail on.
func TestHandleProvider_AddRefusesUnproxyableFlatAPI(t *testing.T) {
	isolateCLI(t)
	cfgPath := os.Getenv("PI_SWITCH_CONFIG")

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"add", "cli-google", "--api", "google-generative-ai"})
	})

	if code == 0 {
		t.Fatalf("provider add with a known-but-unproxyable api exit = 0 (stdout=%q)", out)
	}
	if !strings.Contains(errOut, "google-generative-ai") || !strings.Contains(errOut, "proxy") {
		t.Fatalf("the refusal must name the api and the capability: %q", errOut)
	}
	// 拒绝发生在落盘之前，所以配置可能压根还没被创建。
	if raw, err := os.ReadFile(cfgPath); err == nil && strings.Contains(string(raw), "cli-google") {
		t.Fatal("a refused add still wrote the profile")
	}
}
