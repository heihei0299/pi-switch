package main

import (
	"os"
	"strings"
	"testing"
)

// Review findings (Standards + Spec axes): the three subcommands parsed their
// arguments three different ways and at three different levels of strictness, so
// a typo could be silently swallowed:
//
//   - `fetch-models` ignored every argument after the profile name, so
//     `--channel nope` answered 0 and printed a list as if it had honoured it;
//   - `expose` appended an unknown `--flag` to the model list, and
//     `expose <name>` with no ids WIPED exposedModels while printing success;
//   - `duplicate` reported a valueless trailing `--as` as an unknown argument and
//     silently let a repeated `--as` win.
//
// These tests pin the shared rule: refuse, name the problem, and change nothing.
// The fixtures they use live in provider_cli_helpers_test.go.

// F1: an unknown flag is refused instead of being ignored.
func TestHandleProvider_FetchModelsRejectsUnknownFlag(t *testing.T) {
	isolateCLI(t)
	createOneChannelProfile(t, "p")

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"fetch-models", "p", "--bogus", "x"})
	})

	if code == 0 {
		t.Fatalf("an unknown flag exit = 0 (stdout=%q)", out)
	}
	if !strings.Contains(errOut, "--bogus") {
		t.Fatalf("the offending flag was not named: %q", errOut)
	}
}

// F2: a flag with no value says so, rather than being reported as an unknown
// argument (the wording `provider add` already used).
func TestHandleProvider_DuplicateReportsMissingFlagValue(t *testing.T) {
	isolateCLI(t)
	createOneChannelProfile(t, "p")

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"duplicate", "p", "--as"})
	})

	if code == 0 {
		t.Fatalf("a valueless --as exit = 0 (stdout=%q)", out)
	}
	if !strings.Contains(errOut, "requires a value") {
		t.Fatalf("a valueless flag was not reported as such: %q", errOut)
	}
}

// F3: a repeated flag is refused, and nothing is created behind the user's back.
func TestHandleProvider_DuplicateRejectsRepeatedFlag(t *testing.T) {
	isolateCLI(t)
	createOneChannelProfile(t, "p")

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"duplicate", "p", "--as", "first", "--as", "second"})
	})

	if code == 0 {
		t.Fatalf("a repeated --as exit = 0 (stdout=%q)", out)
	}
	if !strings.Contains(errOut, "--as") {
		t.Fatalf("the repeated flag was not named: %q", errOut)
	}
	for _, name := range []string{"first", "second"} {
		if code, _, _ := runCLIStreams(t, func() int {
			return handleProvider([]string{"show", name})
		}); code == 0 {
			t.Fatalf("a refused duplicate still created profile %q", name)
		}
	}
}

// F4: `expose` must not turn an unknown flag into a model id.
func TestHandleProvider_ExposeRejectsUnknownFlag(t *testing.T) {
	isolateCLI(t)
	createOneChannelProfile(t, "p")

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"expose", "p", "--bogus"})
	})

	if code == 0 {
		t.Fatalf("an unknown flag exit = 0 (stdout=%q)", out)
	}
	if !strings.Contains(errOut, "--bogus") {
		t.Fatalf("the offending flag was not named: %q", errOut)
	}
	if shown := showProfile(t, "p"); strings.Contains(shown, "--bogus") {
		t.Fatalf("the unknown flag was stored as a model id:\n%s", shown)
	}
}

// F5: exposing nothing is a usage error, not a way to silently empty the
// selection. `expose <name>` used to wipe exposedModels and report success.
func TestHandleProvider_ExposeWithoutModelIDsChangesNothing(t *testing.T) {
	isolateCLI(t)
	createOneChannelProfile(t, "p")
	if code, _, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"expose", "p", "m1"})
	}); code != 0 {
		t.Fatalf("setup expose failed: %d %q", code, errOut)
	}
	before := showProfile(t, "p")

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"expose", "p"})
	})

	if code == 0 {
		t.Fatalf("exposing no ids exit = 0 (stdout=%q)", out)
	}
	if !strings.Contains(errOut, "model-id") {
		t.Fatalf("the usage line was not shown: %q", errOut)
	}
	if after := showProfile(t, "p"); after != before {
		t.Fatalf("a refused expose changed the profile:\nbefore=%s\nafter=%s", before, after)
	}
}

// F6: a profile with no channels gets a reason that is true. It used to be told
// it "has multiple channels" while it had none.
func TestHandleProvider_ExposeReportsNoChannelsTruthfully(t *testing.T) {
	isolateCLI(t)
	// A legacy-shaped profile: a top-level baseUrl and no upstreams, which is
	// what `provider add` produced before it learned to create a channel.
	cfgPath := os.Getenv("PI_SWITCH_CONFIG")
	legacy := `{"version":2,"profiles":{"legacy":{"api":"openai-responses","baseUrl":"http://x","apiKey":"k"}},"settings":{}}`
	if err := os.WriteFile(cfgPath, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"expose", "legacy", "m1"})
	})

	if code == 0 {
		t.Fatalf("exposing on a channel-less profile exit = 0 (stdout=%q)", out)
	}
	if strings.Contains(errOut, "multiple channels") {
		t.Fatalf("a profile with no channels was told it has several: %q", errOut)
	}
	if !strings.Contains(errOut, "no channels") {
		t.Fatalf("the real reason was not reported: %q", errOut)
	}
}
