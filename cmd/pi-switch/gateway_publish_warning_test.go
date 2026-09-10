package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// known-gaps/01 (F21), option A, CLI half: `pi-switch gateway publish` writes
// providers whose apiKey is the literal "pi-switch-proxy", which a client sends as
// a Bearer token, and the proxy only accepts HTTP Basic once bound beyond loopback.
// The command used to say only "gateway published".
//
// The warning is derived from the entries being written (their own baseUrl), so it
// cannot drift from the gateway package's own host derivation, and it must never
// carry a credential.

// gatewayPublishConfig writes a config whose published entries are non-empty: one
// profile with one exposed model.
func gatewayPublishConfig(t *testing.T, proxyHost string) {
	t.Helper()
	isolateCLI(t)
	cfgPath := os.Getenv("PI_SWITCH_CONFIG")
	cfg := `{"version":2,"profiles":{"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","models":[],"upstreams":[{"name":"main","baseUrl":"http://a","apiKey":"k","models":[{"id":"m1","contextWindow":100,"maxTokens":10}],"exposedModels":["m1"]}]}},"settings":{"providerPrefix":"pi-switch","gatewayApi":"openai-completions","proxy":{"host":"` + proxyHost + `","port":43112}}}`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_MODELS", filepath.Join(filepath.Dir(cfgPath), "models.json"))
}

// W1: a proxy configured beyond loopback makes the command warn, and still publish.
func TestHandleGatewayCLI_PublishWarnsWhenProxyIsBeyondLoopback(t *testing.T) {
	gatewayPublishConfig(t, "192.168.1.5")

	code, _, errOut := runCLIStreams(t, func() int {
		handleGatewayCLI([]string{"publish"})
		return 0
	})

	// handleGatewayCLI exits the process on failure, so reaching this point means it
	// did not; the side effect is asserted separately.
	if code != 0 {
		t.Fatalf("publish exit = %d (%q)", code, errOut)
	}
	if _, err := os.Stat(os.Getenv("PI_SWITCH_MODELS")); err != nil {
		t.Fatalf("publish did not write models.json: %v", err)
	}
	if !strings.Contains(errOut, "warning:") {
		t.Fatalf("publishing to a proxy at 192.168.1.5 said nothing about authentication: %q", errOut)
	}
	for _, want := range []string{"Basic", "Bearer", "192.168.1.5"} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("the warning does not mention %q: %q", want, errOut)
		}
	}
}

// W2: the default (loopback) deployment stays quiet, and the wildcard host is
// rewritten by the gateway package, not warned about as a remote address.
func TestHandleGatewayCLI_PublishStaysQuietOnLoopback(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "0.0.0.0", ""} {
		t.Run("host="+host, func(t *testing.T) {
			gatewayPublishConfig(t, host)

			_, _, errOut := runCLIStreams(t, func() int {
				handleGatewayCLI([]string{"publish"})
				return 0
			})

			if strings.Contains(errOut, "warning:") {
				t.Fatalf("a loopback publish warned about authentication: %q", errOut)
			}
		})
	}
}

// W3: the warning must not carry the shared password — not publishing it is the
// entire point of option A.
func TestHandleGatewayCLI_PublishWarningCarriesNoCredential(t *testing.T) {
	gatewayPublishConfig(t, "192.168.1.5")
	const secret = "s3cret-shared-password"
	t.Setenv("PI_SWITCH_WEBUI_PASSWORD", secret)

	_, _, errOut := runCLIStreams(t, func() int {
		handleGatewayCLI([]string{"publish"})
		return 0
	})

	if strings.Contains(errOut, secret) {
		t.Fatalf("the warning leaked the shared password: %q", errOut)
	}
	raw, err := os.ReadFile(os.Getenv("PI_SWITCH_MODELS"))
	if err != nil {
		t.Fatalf("models.json not written: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("the shared password was published into models.json: %s", raw)
	}
}
