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

// Ticket 02: the proxy face must hold the same trust boundary as the management
// face, and an unbounded body read must not be reachable from a route that runs
// upstream calls.

func postInference(r http.Handler, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// B1: a loopback proxy stays open — local development and the existing test
// suite must not start demanding credentials. Asserted on a route that answers
// without an upstream call, so the result cannot be confused with an upstream
// rejection (an upstream 401 is text/plain; this middleware answers JSON).
func TestProxyAuth_LoopbackStaysOpen(t *testing.T) {
	cfgPath := isolateConfig(t)
	writeConfigFile(t, cfgPath, "127.0.0.1")
	r := NewProxyRouterWithAuth(MgmtAuthOptions{BindHost: "127.0.0.1"})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /v1/models on loopback = %d, want 200 (%s)", w.Code, w.Body.String())
	}
}

// B1c: the pinned inference path itself stays usable on loopback under the
// default cap — it gets past the body read and the guard, and is only stopped by
// the absent upstream rather than by this change.
func TestProxyAuth_LoopbackInferencePathStaysUsable(t *testing.T) {
	cfgPath := isolateConfig(t)
	writeConfigFile(t, cfgPath, "127.0.0.1")
	r := NewProxyRouterWithAuth(MgmtAuthOptions{BindHost: "127.0.0.1"})

	w := postInference(r, "/v1/chat/completions", `{"model":"gpt-4o-mini"}`, nil)

	// The isolated fixture has no routable profile, so the handler's own
	// envelope is the observable proof that the request got past the read and
	// the guard: 502 no_route, not an auth or size rejection.
	if w.Code != http.StatusBadGateway {
		t.Fatalf("loopback POST /v1/chat/completions = %d (%s), want 502 from the handler envelope", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (%s)", err, w.Body.String())
	}
	if errObj, ok := body["error"].(map[string]any); !ok || errObj["type"] != "no_route" {
		t.Fatalf("body = %s, want error.type=no_route", w.Body.String())
	}
}

// B1b: when the guard does reject, the response is this middleware's JSON 401
// with a challenge — not an upstream error passed through.
func TestProxyAuth_RejectionIsTheMiddlewareAnswer(t *testing.T) {
	cfgPath := isolateConfig(t)
	writeConfigFile(t, cfgPath, "127.0.0.1")
	r := NewProxyRouterWithAuth(MgmtAuthOptions{BindHost: "0.0.0.0", Password: testPassword})

	w := postInference(r, "/v1/chat/completions", `{"model":"gpt-4o-mini"}`, nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", w.Code)
	}
	if got := w.Header().Get("WWW-Authenticate"); got == "" {
		t.Fatal("401 has no WWW-Authenticate challenge, so it may not be the middleware's answer")
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON (an upstream rejection would be text/plain)", ct)
	}
}

// B2: an exposed proxy without credentials must refuse the inference routes.
func TestProxyAuth_NonLoopbackWithoutPasswordIsRejected(t *testing.T) {
	cfgPath := isolateConfig(t)
	writeConfigFile(t, cfgPath, "127.0.0.1") // config keeps saying loopback
	r := NewProxyRouterWithAuth(MgmtAuthOptions{BindHost: "0.0.0.0"})

	cases := []struct{ method, path string }{
		{http.MethodPost, "/v1/chat/completions"},
		{http.MethodGet, "/v1/models"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s with bind host 0.0.0.0 and no password = %d, want 401", tc.method, tc.path, w.Code)
		}
	}
}

// B3: the proxy uses the same credential the management API uses, and a wrong
// one is refused.
func TestProxyAuth_UsesTheSharedCredential(t *testing.T) {
	cfgPath := isolateConfig(t)
	writeConfigFile(t, cfgPath, "127.0.0.1")
	r := NewProxyRouterWithAuth(MgmtAuthOptions{BindHost: "0.0.0.0", Password: testPassword})

	wrong := postInference(r, "/v1/chat/completions", `{"model":"gpt-4o-mini"}`,
		map[string]string{"Authorization": basicAuthHeader("admin", "wrong")})
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong credential = %d, want 401", wrong.Code)
	}

	right := postInference(r, "/v1/chat/completions", `{"model":"gpt-4o-mini"}`,
		map[string]string{"Authorization": basicAuthHeader("admin", testPassword)})
	if right.Code == http.StatusUnauthorized {
		t.Fatalf("correct credential rejected: %d (%s)", right.Code, right.Body.String())
	}
}

// B4: health probes stay reachable on an exposed proxy so a misconfigured
// listener remains diagnosable.
func TestProxyAuth_HealthProbeStaysReachable(t *testing.T) {
	isolateConfig(t)
	r := NewProxyRouterWithAuth(MgmtAuthOptions{BindHost: "0.0.0.0", Password: testPassword})

	for _, path := range []string{"/healthz", "/health"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200 without credentials", path, w.Code)
		}
	}
}

// B5: a body over the cap is refused with 413 rather than buffered.
func TestProxyBodyCap_RejectsOversizedBody(t *testing.T) {
	cfgPath := isolateConfig(t)
	writeConfigFile(t, cfgPath, "127.0.0.1")
	t.Setenv("PI_SWITCH_MAX_BODY_BYTES", "1024")
	r := NewProxyRouterWithAuth(MgmtAuthOptions{BindHost: "127.0.0.1"})

	huge := `{"model":"gpt-4o-mini","padding":"` + strings.Repeat("x", 4096) + `"}`
	w := postInference(r, "/v1/chat/completions", huge, nil)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body = %d, want 413 (%s)", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode 413 body: %v (%s)", err, w.Body.String())
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("413 body = %s, want an error object", w.Body.String())
	}
	if errObj["type"] != "request_too_large" {
		t.Fatalf("error.type = %v, want request_too_large", errObj["type"])
	}
}

// B6: the cap is what the configuration says, and it defaults to 32 MiB.
func TestProxyBodyCap_ConfigurationAndDefault(t *testing.T) {
	t.Setenv("PI_SWITCH_MAX_BODY_BYTES", "")
	// 33554432 is 32 MiB stated independently of how the code computes it.
	if got := maxProxyBodyBytes(); got != 33554432 {
		t.Fatalf("default cap = %d, want 33554432", got)
	}
	t.Setenv("PI_SWITCH_MAX_BODY_BYTES", "4096")
	if got := maxProxyBodyBytes(); got != 4096 {
		t.Fatalf("configured cap = %d, want 4096", got)
	}
	// A nonsensical value must not silently disable the cap.
	for _, bad := range []string{"0", "-1", "not-a-number"} {
		t.Setenv("PI_SWITCH_MAX_BODY_BYTES", bad)
		if got := maxProxyBodyBytes(); got != 33554432 {
			t.Fatalf("cap for %q = %d, want the default", bad, got)
		}
	}
}

// B7: a request refused by the cap must leave no request fact behind and must
// never reach the upstream. Both the refusal and the control are measured on the
// SAME router and fixture, so the contrast is attributable to body size alone.
func TestProxyBodyCap_DoesNotRecordRejectedRequest(t *testing.T) {
	cfgPath := isolateConfig(t)
	writeConfigFile(t, cfgPath, "127.0.0.1")
	t.Setenv("PI_SWITCH_MAX_BODY_BYTES", "1024")

	var upstreamHits int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer upstream.Close()

	// resolveRoute matches the channel's exposedModels, so the fixture declares
	// an upstream channel that exposes the model.
	writeChannelConfig(t, filepath.Dir(os.Getenv("PI_SWITCH_DB")), `{"version":2,"profiles":{"sup":{"api":"openai-responses","responsesMode":"passthrough","baseUrl":"`+upstream.URL+`","apiKey":"k","upstreams":[{"name":"main","api":"openai-responses","baseUrl":"`+upstream.URL+`","apiKey":"k","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"exposedModels":["gpt-4o-mini"]}]}},"settings":{}}`)
	r := NewProxyRouterWithAuth(MgmtAuthOptions{BindHost: "127.0.0.1"})

	huge := `{"model":"gpt-4o-mini","padding":"` + strings.Repeat("y", 4096) + `"}`
	if w := postInference(r, "/v1/chat/completions", huge, nil); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body = %d, want 413", w.Code)
	}
	if upstreamHits != 0 {
		t.Fatalf("a capped request reached the upstream %d time(s)", upstreamHits)
	}
	if got := totalRequests(t); got != 0 {
		t.Fatalf("totalRequests = %d, want 0 (the rejected request must not be recorded)", got)
	}

	// Positive control on the same router and fixture: a normal request does get
	// recorded, so the zero above is not a broken probe.
	normal := postInference(r, "/v1/chat/completions", `{"model":"gpt-4o-mini"}`, nil)
	if normal.Code == http.StatusRequestEntityTooLarge {
		t.Fatalf("a small body was rejected by the cap: %d", normal.Code)
	}
	if got := totalRequests(t); got != 1 {
		t.Fatalf("totalRequests after a routed request = %d, want 1", got)
	}
}

func totalRequests(t *testing.T) int {
	t.Helper()
	// The proxy face has no stats route; read the count through the management
	// face, which shares the same request database.
	w := getAPI(NewMgmtRouter(), "/api/stats", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/stats = %d (%s)", w.Code, w.Body.String())
	}
	var stats map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("decode stats: %v (%s)", err, w.Body.String())
	}
	got, ok := stats["totalRequests"].(float64)
	if !ok {
		t.Fatalf("totalRequests missing: %s", w.Body.String())
	}
	return int(got)
}
