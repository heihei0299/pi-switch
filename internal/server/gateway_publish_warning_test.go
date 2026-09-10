package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// known-gaps/01 (F21), option A: the providers pi-switch publishes carry
// `"apiKey": "pi-switch-proxy"` (gateway.go:236) with a baseUrl pointing at the
// proxy (/v1), and a client sends that key as a Bearer token — while the proxy
// guard only accepts HTTP Basic (server.go:486). On a listener that is not
// loopback the guard is always installed, because ValidateBindAuth refuses to
// start otherwise. So the published providers cannot authenticate there, and the
// operator is never told.
//
// Option A (chosen): say so plainly, publish nothing secret. These tests pin both
// halves — the warning on a guarded listener, and silence on loopback so the
// default deployment is not spammed.

func publishableConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	cfgJSON := `{"version":2,"profiles":{
		"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","models":[],"upstreams":[
			{"name":"main","baseUrl":"http://a","apiKey":"k","models":[{"id":"m1","contextWindow":100,"maxTokens":10}],"exposedModels":["m1"]}]}},
		"settings":{"providerPrefix":"pi-switch","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112}}}`
	writeChannelConfig(t, dir, cfgJSON)
	t.Setenv("PI_SWITCH_MODELS", filepath.Join(dir, "models.json"))
}

const publishPayload = `{"providers":{"pi-switch-chat":{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","proxy":false,"models":[{"id":"m1","contextWindow":100,"maxTokens":10}]}}}`

func publishVia(t *testing.T, r http.Handler, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(publishPayload))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func warningsOf(t *testing.T, w *httptest.ResponseRecorder) []string {
	t.Helper()
	var body struct {
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v (%s)", err, w.Body.String())
	}
	return body.Warnings
}

// H1: publishing on a guarded listener warns that the published provider cannot
// authenticate, and names both sides of the mismatch.
func TestGatewayPublish_WarnsWhenPublishedProviderCannotAuthenticate(t *testing.T) {
	publishableConfig(t)
	r := NewMgmtRouterWithAuth(MgmtAuthOptions{BindHost: "0.0.0.0", Password: testPassword})
	auth := map[string]string{"Authorization": basicAuthHeader("admin", testPassword)}

	for _, tc := range []struct{ method, path string }{
		{http.MethodPut, "/api/models/gateway"},
		{http.MethodPost, "/api/gateway/publish"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			w := publishVia(t, r, tc.method, tc.path, auth)
			if w.Code != http.StatusOK {
				t.Fatalf("publish = %d (%s), want 200", w.Code, w.Body.String())
			}
			warnings := warningsOf(t, w)
			if len(warnings) == 0 {
				t.Fatalf("publishing to a guarded listener said nothing about authentication: %s", w.Body.String())
			}
			joined := strings.Join(warnings, " ")
			for _, want := range []string{"Basic", "Bearer"} {
				if !strings.Contains(joined, want) {
					t.Fatalf("the warning does not mention %q: %q", want, joined)
				}
			}
		})
	}
}

// H2: loopback (the default deployment) stays quiet — the warning is about a real
// limitation, not decoration.
func TestGatewayPublish_StaysQuietOnLoopback(t *testing.T) {
	publishableConfig(t)
	r := NewMgmtRouter()

	w := publishVia(t, r, http.MethodPut, "/api/models/gateway", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("publish = %d (%s), want 200", w.Code, w.Body.String())
	}
	if warnings := warningsOf(t, w); len(warnings) != 0 {
		t.Fatalf("a loopback publish warned about authentication: %v", warnings)
	}
}

// H3: the warning must never carry a credential. Option A exists precisely to
// avoid publishing the shared password into ~/.pi/agent/models.json, and the same
// rule holds for anything else the API hands back.
func TestGatewayPublish_WarningCarriesNoCredential(t *testing.T) {
	publishableConfig(t)
	r := NewMgmtRouterWithAuth(MgmtAuthOptions{BindHost: "0.0.0.0", Password: testPassword})

	w := publishVia(t, r, http.MethodPut, "/api/models/gateway", map[string]string{"Authorization": basicAuthHeader("admin", testPassword)})
	if w.Code != http.StatusOK {
		t.Fatalf("publish = %d (%s), want 200", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, testPassword) {
		t.Fatalf("the response leaked the shared password: %s", body)
	}
	// And nothing was written into the published file either.
	raw, err := os.ReadFile(os.Getenv("PI_SWITCH_MODELS"))
	if err != nil {
		t.Fatalf("models.json not written: %v", err)
	}
	if strings.Contains(string(raw), testPassword) {
		t.Fatalf("the shared password was published into models.json: %s", raw)
	}
}
