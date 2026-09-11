package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

// F21 option A, corrected after review (review-remediation/01).
//
// The providers pi-switch publishes carry `"apiKey": "pi-switch-proxy"`, which a
// client sends as a Bearer token, while the proxy's /v1 surface requires HTTP Basic
// whenever it is bound beyond loopback (a bind beyond loopback always has a
// password: ValidateBindAuth refuses to start without one).
//
// So the caveat must be judged from the PROXY's exposure. The first version judged
// the bind address of the listener answering the request, and the CLI version judged
// the published baseUrl — which BuildProposedGatewayEntry rewrites to 127.0.0.1 for
// wildcard binds. Both were silent for a proxy on 0.0.0.0, whose published provider
// answers 401.

func publishableConfigWithProxyHost(t *testing.T, proxyHost string) {
	t.Helper()
	dir := t.TempDir()
	cfgJSON := `{"version":2,"profiles":{
		"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","models":[],"upstreams":[
			{"name":"main","baseUrl":"http://a","apiKey":"k","models":[{"id":"m1","contextWindow":100,"maxTokens":10}],"exposedModels":["m1"]}]}},
		"settings":{"providerPrefix":"pi-switch","gatewayApi":"openai-completions","proxy":{"host":"` + proxyHost + `","port":43112}}}`
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

func assertAuthWarning(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	warnings := warningsOf(t, w)
	if len(warnings) == 0 {
		t.Fatalf("an exposed proxy produced no authentication warning: %s", w.Body.String())
	}
	joined := strings.Join(warnings, " ")
	for _, want := range []string{"Basic", "Bearer"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("the warning does not mention %q: %q", want, joined)
		}
	}
}

// H1: an exposed proxy warns, on BOTH publish routes, whatever this listener is bound
// to — including a wildcard host, whose published URL is rewritten to 127.0.0.1 so
// that the plan alone reads as local.
//
// The empty host is deliberately NOT in this table: config.LoadConfigAtPath
// normalizes an empty settings.proxy.host to 127.0.0.1 (config.go:151/198/388), so the
// handler cannot see it. An empty host means "every interface" only as a *runtime*
// bind address (--host ""), which the config cannot express — that case is the
// documented residual limit of this caveat.
func TestGatewayPublish_WarnsWhenTheProxyIsExposed(t *testing.T) {
	for _, proxyHost := range []string{"0.0.0.0", "192.168.1.5", "::"} {
		for _, route := range []struct{ method, path string }{
			{http.MethodPut, "/api/models/gateway"},
			{http.MethodPost, "/api/gateway/publish"},
		} {
			t.Run("proxy.host="+proxyHost+" "+route.path, func(t *testing.T) {
				publishableConfigWithProxyHost(t, proxyHost)
				r := NewMgmtRouter() // loopback listener on purpose: see H2

				w := publishVia(t, r, route.method, route.path, nil)
				if w.Code != http.StatusOK {
					t.Fatalf("publish = %d (%s), want 200", w.Code, w.Body.String())
				}
				assertAuthWarning(t, w)
			})
		}
	}
}

// H2: a loopback proxy stays quiet even when this listener is bound beyond loopback.
// Where the management API listens says nothing about whether the published provider
// can authenticate — a loopback webui with an exposed proxy is a supported pairing,
// and warning there would be noise.
func TestGatewayPublish_StaysQuietWhenTheProxyIsLoopback(t *testing.T) {
	publishableConfigWithProxyHost(t, "127.0.0.1")
	r := NewMgmtRouterWithAuth(MgmtAuthOptions{BindHost: "0.0.0.0", Password: testPassword})

	w := publishVia(t, r, http.MethodPut, "/api/models/gateway", map[string]string{"Authorization": basicAuthHeader("admin", testPassword)})
	if w.Code != http.StatusOK {
		t.Fatalf("publish = %d (%s), want 200", w.Code, w.Body.String())
	}
	if warnings := warningsOf(t, w); len(warnings) != 0 {
		t.Fatalf("a loopback proxy warned about authentication: %v", warnings)
	}
}

// H3: the caveat also reads the entries themselves, so a hand-edited plan that points
// clients at a LAN host is judged even when the configured proxy is loopback. This is
// the exported seam the CLI shares; the HTTP cases above cannot reach it.
func TestPublishedAuthCaveat_JudgesThePlanToo(t *testing.T) {
	cases := []struct {
		name      string
		cfg       config.PiSwitchConfig
		published map[string]interface{}
		want      bool
	}{
		{"loopback proxy, loopback plan", configWithProxyHost("127.0.0.1"), planWithBaseURL("http://127.0.0.1:43112/v1"), false},
		{"loopback proxy, LAN plan", configWithProxyHost("127.0.0.1"), planWithBaseURL("http://192.168.1.9:43112/v1"), true},
		{"loopback proxy, no plan", configWithProxyHost("127.0.0.1"), nil, false},
		{"wildcard proxy", configWithProxyHost("0.0.0.0"), planWithBaseURL("http://127.0.0.1:43112/v1"), true},
		{"empty proxy host", configWithProxyHost(""), planWithBaseURL("http://127.0.0.1:43112/v1"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := PublishedAuthCaveat(tc.cfg, tc.published) != ""
			if got != tc.want {
				t.Fatalf("caveat present = %v, want %v", got, tc.want)
			}
		})
	}
}

func configWithProxyHost(host string) config.PiSwitchConfig {
	var cfg config.PiSwitchConfig
	cfg.Settings.Proxy.Host = host
	return cfg
}

// planWithBaseURL builds a hand-edited plan that a client can actually call: one
// pi-switch provider carrying one model. An empty model list would be unroutable, and
// the caveat deliberately stays quiet about those (H7).
func planWithBaseURL(base string) map[string]interface{} {
	return map[string]interface{}{
		"providers": map[string]interface{}{
			"pi-switch-chat": map[string]interface{}{
				"api": "openai-completions", "baseUrl": base, "apiKey": "pi-switch-proxy",
				"models": []interface{}{map[string]interface{}{"id": "m1"}},
			},
		},
	}
}

// H4: the warning must never carry a credential — not publishing the shared password
// into models.json is the entire point of option A.
func TestGatewayPublish_WarningCarriesNoCredential(t *testing.T) {
	publishableConfigWithProxyHost(t, "0.0.0.0")
	r := NewMgmtRouterWithAuth(MgmtAuthOptions{BindHost: "0.0.0.0", Password: testPassword})

	w := publishVia(t, r, http.MethodPut, "/api/models/gateway", map[string]string{"Authorization": basicAuthHeader("admin", testPassword)})
	if w.Code != http.StatusOK {
		t.Fatalf("publish = %d (%s), want 200", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), testPassword) {
		t.Fatalf("the response leaked the shared password: %s", w.Body.String())
	}
	raw, err := os.ReadFile(os.Getenv("PI_SWITCH_MODELS"))
	if err != nil {
		t.Fatalf("models.json not written: %v", err)
	}
	if strings.Contains(string(raw), testPassword) {
		t.Fatalf("the shared password was published into models.json: %s", raw)
	}
}

// H5: the hazard is "a client reads one of OUR providers out of models.json and calls
// the exposed proxy". A plan that publishes no pi-switch provider at all cannot produce
// that 401 — here nothing is exposed, so the only entry is a hand-kept third-party
// provider — and warning about it would be noise that also misdescribes what was
// published.
func TestGatewayPublish_StaysQuietWithoutOwnProviders(t *testing.T) {
	publishableConfigWithoutExposedModels(t, "0.0.0.0")
	r := NewMgmtRouter()

	// Empty body: the generated flow, whose plan is built from config alone.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/publish", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("publish = %d (%s), want 200", w.Code, w.Body.String())
	}
	if warnings := warningsOf(t, w); len(warnings) != 0 {
		t.Fatalf("a plan publishing no pi-switch provider warned about authentication: %v", warnings)
	}
	// The plan was not empty: the third-party entry really was published, so the
	// silence above is the judgement, not a publish that did nothing.
	raw, err := os.ReadFile(os.Getenv("PI_SWITCH_MODELS"))
	if err != nil {
		t.Fatalf("models.json not written: %v", err)
	}
	if !strings.Contains(string(raw), "third-party") {
		t.Fatalf("the third-party entry was not published: %s", raw)
	}
	for _, key := range []string{"pi-switch-chat", "pi-switch-res"} {
		if strings.Contains(string(raw), `"`+key+`"`) {
			t.Fatalf("the fixture published %s: %s", key, raw)
		}
	}
}

// H7: a fixed provider published with an empty model list is unroutable —
// ValidateGatewayProvider accepts `"models": []` — so no client can call it and the
// 401 cannot happen. The gate must look at models, not just at the provider key.
func TestGatewayPublish_StaysQuietForAnEmptyFixedProvider(t *testing.T) {
	publishableConfigWithProxyHost(t, "0.0.0.0")
	r := NewMgmtRouter()

	payload := `{"providers":{"pi-switch-chat":{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","proxy":false,"models":[]}}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/models/gateway", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("publish = %d (%s), want 200", w.Code, w.Body.String())
	}
	if warnings := warningsOf(t, w); len(warnings) != 0 {
		t.Fatalf("an unroutable fixed provider warned about authentication: %v", warnings)
	}
}

// H6: the judgement does not read the apiKey value. A hand-edited fixed provider with a
// real key reaches the client as `Authorization: Bearer <that key>`, and the exposed
// proxy still answers 401, so it must warn exactly like the placeholder does.
func TestGatewayPublish_WarnsWhateverTheFixedEntryKeySays(t *testing.T) {
	publishableConfigWithProxyHost(t, "0.0.0.0")
	retargeted := strings.Replace(publishPayload, `"apiKey":"pi-switch-proxy"`, `"apiKey":"sk-real"`, 1)
	if retargeted == publishPayload {
		t.Fatal("fixture changed: publishPayload no longer carries the placeholder key")
	}
	r := NewMgmtRouter()

	req := httptest.NewRequest(http.MethodPut, "/api/models/gateway", strings.NewReader(retargeted))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("publish = %d (%s), want 200", w.Code, w.Body.String())
	}
	assertAuthWarning(t, w)
}

// publishableConfigWithoutExposedModels is publishableConfigWithProxyHost with zero
// exposure, plus the third-party provider a real models.json may already carry: the
// generated plan then publishes that entry and none of pi-switch's own.
func publishableConfigWithoutExposedModels(t *testing.T, proxyHost string) {
	t.Helper()
	dir := t.TempDir()
	cfgJSON := `{"version":2,"profiles":{
		"sup":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://x","apiKey":"k","upstreams":[
			{"name":"main","baseUrl":"http://a","apiKey":"k","models":[{"id":"m1","contextWindow":100,"maxTokens":10}]}]}},
		"settings":{"providerPrefix":"pi-switch","gatewayApi":"openai-completions","proxy":{"host":"` + proxyHost + `","port":43112}}}`
	writeChannelConfig(t, dir, cfgJSON)
	modelsPath := filepath.Join(dir, "models.json")
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	if err := os.WriteFile(modelsPath, []byte(`{"providers":{"third-party":{"api":"openai-completions","baseUrl":"https://third.example/v1","apiKey":"sk-real","models":[{"id":"m1"}]}}}`), 0644); err != nil {
		t.Fatal(err)
	}
}
