package main

import (
	"strings"
	"testing"
)

// Ticket provider-cli 05: `provider expose` was documented in both READMEs but
// answered "unknown provider subcommand". The server implements it
// (PUT /api/profiles/:name/expose); the CLI is now wired to the same core.
//
// The fixtures live in provider_cli_helpers_test.go. They used to build a mock
// upstream that the expose path never contacted (the channel's models came from
// --models), so the mock only donated a URL — a helper asserting nothing.

// B1: exposing a model the channel carries is recorded in the config, as the CLI
// itself reports it.
func TestHandleProvider_ExposeRecordsSelection(t *testing.T) {
	isolateCLI(t)
	createOneChannelProfile(t, "exp")

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"expose", "exp", "m1"})
	})
	if code != 0 {
		t.Fatalf("provider expose exit = %d (stdout=%q stderr=%q)", code, out, errOut)
	}
	if exposed := shownChannelExposed(t, "exp", "main"); len(exposed) != 1 || exposed[0] != "m1" {
		t.Fatalf("exposedModels = %v, want [m1]", exposed)
	}
}

// B2: exposing an id the channel does not carry is refused — otherwise routing
// would advertise a model nothing can serve.
func TestHandleProvider_ExposeRefusesUnknownModel(t *testing.T) {
	isolateCLI(t)
	createOneChannelProfile(t, "exp")

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
