package main

import (
	"os"
	"strings"
	"testing"
)

// Ticket provider-cli 02: `provider duplicate` must copy a profile for real. It
// used to answer "unknown provider subcommand" while the help text advertised it
// and POST /api/profiles/:name/duplicate implemented it.

// B1: duplicate creates a copy that a later list shows, with the source's内容.
func TestHandleProvider_DuplicateCreatesCopy(t *testing.T) {
	isolateCLI(t)
	cfgPath := os.Getenv("PI_SWITCH_CONFIG")

	if code, _, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"add", "src", "--base-url", "http://127.0.0.1:9/v1", "--api-key", "k1"})
	}); code != 0 {
		t.Fatalf("setup add failed: %d %q", code, errOut)
	}

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"duplicate", "src", "--as", "copy"})
	})
	if code != 0 {
		t.Fatalf("provider duplicate exit = %d (stdout=%q stderr=%q)", code, out, errOut)
	}

	profiles := readProfiles(t, cfgPath)
	orig, ok := profiles["src"].(map[string]any)
	if !ok {
		t.Fatalf("source profile missing after duplicate: %v", profiles)
	}
	dup, ok := profiles["copy"].(map[string]any)
	if !ok {
		t.Fatalf("copy was not created; config has %v", profiles)
	}
	if dup["baseUrl"] != orig["baseUrl"] || dup["apiKey"] != orig["apiKey"] {
		t.Fatalf("copy differs from source: copy=%v source=%v", dup, orig)
	}

	code, out = runCLI(t, func() int { return handleProvider([]string{"list"}) })
	if code != 0 || !strings.Contains(out, "copy") {
		t.Fatalf("provider list does not show the copy: exit=%d out=%q", code, out)
	}
}

// B2: --as is required.
func TestHandleProvider_DuplicateRequiresAs(t *testing.T) {
	isolateCLI(t)

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"duplicate", "src"})
	})

	if code == 0 {
		t.Fatalf("duplicate without --as exit = 0 (stdout=%q)", out)
	}
	if !strings.Contains(errOut, "--as") {
		t.Fatalf("duplicate without --as did not explain the requirement: %q", errOut)
	}
}

// B3: an unknown source and an existing target are both refused, and neither
// modifies the config.
func TestHandleProvider_DuplicateRefusesBadInputs(t *testing.T) {
	isolateCLI(t)
	cfgPath := os.Getenv("PI_SWITCH_CONFIG")

	if code, _, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"add", "one", "--base-url", "http://127.0.0.1:9/v1"})
	}); code != 0 {
		t.Fatalf("setup add failed: %d %q", code, errOut)
	}
	if code, _, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"add", "two", "--base-url", "http://127.0.0.1:9/v1"})
	}); code != 0 {
		t.Fatalf("setup add failed: %d %q", code, errOut)
	}
	before, _ := os.ReadFile(cfgPath)

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"unknown source", []string{"duplicate", "nope", "--as", "x"}, "not found"},
		{"existing target", []string{"duplicate", "one", "--as", "two"}, "exists"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := runCLIStreams(t, func() int { return handleProvider(tc.args) })
			if code == 0 {
				t.Fatalf("%s exit = 0 (stdout=%q)", tc.name, out)
			}
			if !strings.Contains(errOut, tc.want) {
				t.Fatalf("%s did not explain the failure (%q): %q", tc.name, tc.want, errOut)
			}
		})
	}
	after, _ := os.ReadFile(cfgPath)
	if string(before) != string(after) {
		t.Fatal("a refused duplicate still modified the config")
	}
}
