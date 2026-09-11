package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Ticket 04: no management endpoint may answer 200 while claiming a side effect
// it never performs. Endpoints without an implementation answer 501 with the
// management error envelope (a bare message; system-contract 2.8); endpoints whose
// empty result is the truth keep answering 200.

func mustConfigPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfgPath := dir + "/config.json"
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dir+"/requests.db")
	return cfgPath
}

func callMgmt(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func assertNotImplemented(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("code = %d (%s), want 501", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (%s)", err, w.Body.String())
	}
	// Management surface (system-contract 2.8): the 501 status carries the
	// "not implemented" kind, so error is a bare message the WebUI can render.
	msg, ok := body["error"].(string)
	if !ok || msg == "" {
		t.Fatalf("body = %s, want a non-empty string error", w.Body.String())
	}
	// A 501 must not also claim success.
	if _, ok := body["ok"]; ok {
		t.Fatalf("not_implemented body also carries ok: %s", w.Body.String())
	}
}

// B1: every endpoint that asserted a side effect it does not perform now
// refuses with 501.
func TestStubEndpoints_RefuseSideEffectClaims(t *testing.T) {
	mustConfigPath(t)
	r := NewMgmtRouter()

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/config/export", `{"passphrase":"x"}`},
		{http.MethodPost, "/api/config/import", `{"filePath":"/tmp/x","passphrase":"y"}`},
		{http.MethodPost, "/api/config/restore", `{"backupPath":"/tmp/x"}`},
		{http.MethodGet, "/api/backups", ""},
		{http.MethodPost, "/api/ccswitch/import", `{}`},
		{http.MethodPost, "/api/init", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			assertNotImplemented(t, callMgmt(r, tc.method, tc.path, tc.body))
		})
	}
}

// B2: an empty collection is the truth, not a fake success, so the endpoint
// keeps 200 and its payload shape.
func TestEmptyCollectionEndpoint_StaysOK(t *testing.T) {
	mustConfigPath(t)
	r := NewMgmtRouter()

	providers := callMgmt(r, http.MethodGet, "/api/ccswitch/providers", "")
	if providers.Code != http.StatusOK {
		t.Fatalf("GET /api/ccswitch/providers = %d, want 200 (an empty list is the truth)", providers.Code)
	}
	var pBody map[string]any
	if err := json.Unmarshal(providers.Body.Bytes(), &pBody); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	if _, ok := pBody["providers"]; !ok {
		t.Fatalf("providers payload lost its providers field: %s", providers.Body.String())
	}
}
