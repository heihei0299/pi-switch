package main

import (
	"strings"
	"testing"
)

// Ticket provider-cli 03: `provider test` must really probe the upstream. The
// server handler does a read-only GET (with a 5s timeout) and reports
// success/message/responseTimeMs; the CLI answered "unknown provider
// subcommand" while the help text advertised it.

// B1: a profile whose upstream cannot be reached reports failure — the probe is
// real, not a stub that always succeeds (the handler was once exactly that).
func TestHandleProvider_TestReportsUnreachable(t *testing.T) {
	isolateCLI(t)

	if code, _, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"add", "dead", "--base-url", "http://127.0.0.1:9/v1"})
	}); code != 0 {
		t.Fatalf("setup add failed: %d %q", code, errOut)
	}

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"test", "dead"})
	})

	if code == 0 {
		t.Fatalf("testing an unreachable upstream exit = 0; a probe that always succeeds is the bug this guards (stdout=%q)", out)
	}
	if !strings.Contains(errOut, "unreachable") {
		t.Fatalf("failure reason missing from stderr: %q", errOut)
	}
	if strings.Contains(out, "ok") && !strings.Contains(errOut, "unreachable") {
		t.Fatalf("stdout claims success: %q", out)
	}
}

// B2: an unknown profile is refused before any network attempt.
func TestHandleProvider_TestRejectsUnknownProfile(t *testing.T) {
	isolateCLI(t)

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"test", "nope"})
	})

	if code == 0 {
		t.Fatalf("testing an unknown profile exit = 0 (stdout=%q)", out)
	}
	// The CLI's convention for this is "unknown profile" (same as show/use/delete);
	// the server handler words it "not found". Assert the CLI's own wording.
	if !strings.Contains(errOut, "unknown profile") {
		t.Fatalf("unknown profile was not reported: %q", errOut)
	}
}

// B3: a profile with no baseUrl is reported as a finding, not as a crash or a
// silent pass. (The handler answers success:false with an explanation.)
func TestHandleProvider_TestReportsEmptyBaseURL(t *testing.T) {
	isolateCLI(t)

	if code, _, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"add", "empty"})
	}); code != 0 {
		t.Fatalf("setup add failed: %d %q", code, errOut)
	}

	code, out, errOut := runCLIStreams(t, func() int {
		return handleProvider([]string{"test", "empty"})
	})

	if code == 0 {
		t.Fatalf("testing a profile with no baseUrl exit = 0 (stdout=%q)", out)
	}
	if !strings.Contains(errOut, "baseUrl") {
		t.Fatalf("empty baseUrl was not explained: %q", errOut)
	}
}
