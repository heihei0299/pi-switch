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
	// "0.0.0.0" and "" are the review's counter-example: the published baseUrl is
	// rewritten to 127.0.0.1, so a check that reads only the plan stays silent while
	// the proxy really is exposed (and its published provider answers 401).
	// The empty host is absent on purpose: the config loader normalizes it to
	// 127.0.0.1, so it cannot express "every interface" here (only a runtime --host ""
	// can, which the config does not record).
	for _, host := range []string{"192.168.1.5", "0.0.0.0", "::"} {
		t.Run("proxy.host="+host, func(t *testing.T) {
			gatewayPublishConfig(t, host)

			_, _, errOut := runCLIStreams(t, func() int {
				handleGatewayCLI([]string{"publish"})
				return 0
			})

			if _, err := os.Stat(os.Getenv("PI_SWITCH_MODELS")); err != nil {
				t.Fatalf("publish did not write models.json: %v", err)
			}
			if !strings.Contains(errOut, "warning:") {
				t.Fatalf("publishing for a proxy at %q said nothing about authentication: %q", host, errOut)
			}
			for _, want := range []string{"Basic", "Bearer"} {
				if !strings.Contains(errOut, want) {
					t.Fatalf("the warning does not mention %q: %q", want, errOut)
				}
			}
		})
	}
}

// W2: a loopback proxy stays quiet. The wildcard and empty hosts used to be listed
// here as "loopback" — that reading was wrong (they listen on every interface) and it
// hid the case the ticket is about, so they now belong to W1.
func TestHandleGatewayCLI_PublishStaysQuietOnLoopback(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1"} {
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
	gatewayPublishConfig(t, "0.0.0.0")
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
