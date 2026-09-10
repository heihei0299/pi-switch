package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Auth seam: the management router takes the *actual bind address* it will
// listen on. Nothing may re-derive that from the config file, because the bind
// address comes from the CLI flag and is never written back to config.

const testPassword = "s3cret"

func basicAuthHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func getAPI(r http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// isolateConfig keeps these tests off the developer's real ~/.pi-switch state,
// matching the package norm (server_test.go, channel_validation_test.go).
func isolateConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	t.Setenv("PI_SWITCH_WEBUI_PASSWORD_FILE", filepath.Join(dir, "webui_password"))
	return cfgPath
}

// writeConfigFile declares a web host in the config file. Every test asserting
// an *exposed* posture writes loopback here, so the test fails if the code goes
// back to trusting cfg.Settings.Web.Host instead of the bind address.
func writeConfigFile(t *testing.T, path, webHost string) {
	t.Helper()
	body := `{"version":2,"profiles":{},"settings":{"web":{"host":"` + webHost + `","port":43110}}}`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func authRequired(t *testing.T, r http.Handler, headers map[string]string) bool {
	t.Helper()
	w := getAPI(r, "/api/webui/info", headers)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/webui/info = %d (%s), want 200", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode /api/webui/info: %v (%s)", err, w.Body.String())
	}
	got, ok := body["authRequired"].(bool)
	if !ok {
		t.Fatalf("authRequired missing or not a bool: %s", w.Body.String())
	}
	return got
}

// B1: a loopback bind stays open — local development must not start demanding
// credentials it never had.
func TestMgmtAuth_LoopbackWithoutPasswordServes(t *testing.T) {
	cfgPath := isolateConfig(t)
	writeConfigFile(t, cfgPath, "127.0.0.1")
	r := NewMgmtRouterWithAuth(MgmtAuthOptions{BindHost: "127.0.0.1"})

	w := getAPI(r, "/api/config", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/config on loopback = %d, want 200", w.Code)
	}
}

// B2: binding a non-loopback address with no password must not serve the
// management API. The config file still says 127.0.0.1, as it does in
// production because --host is never written back, so this fails against a
// handler that trusts the config file.
func TestMgmtAuth_NonLoopbackWithoutPasswordIsRejected(t *testing.T) {
	cfgPath := isolateConfig(t)
	writeConfigFile(t, cfgPath, "127.0.0.1")
	r := NewMgmtRouterWithAuth(MgmtAuthOptions{BindHost: "0.0.0.0"})

	w := getAPI(r, "/api/config", nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/config with bind host 0.0.0.0 and no password = %d, want 401", w.Code)
	}
}

// B3: with a password configured, only the correct admin credential is served.
func TestMgmtAuth_NonLoopbackRequiresCorrectCredential(t *testing.T) {
	isolateConfig(t)
	r := NewMgmtRouterWithAuth(MgmtAuthOptions{BindHost: "0.0.0.0", Password: testPassword})

	served := getAPI(r, "/api/config", map[string]string{"Authorization": basicAuthHeader("admin", testPassword)})
	if served.Code != http.StatusOK {
		t.Fatalf("correct credential = %d, want 200", served.Code)
	}

	denied := getAPI(r, "/api/config", map[string]string{"Authorization": basicAuthHeader("admin", "wrong")})
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("wrong credential = %d, want 401", denied.Code)
	}
}

// B4: wildcard binds expose every interface, so they must be classified as
// non-loopback. `0.0.0.0` used to be listed as loopback, which is how the
// management API ended up open to the network. An empty host belongs to the
// same class: gin's Run turns ":43110" into a wildcard bind, so treating "" as
// local failed open.
func TestMgmtAuth_WildcardBindsAreNotLoopback(t *testing.T) {
	cfgPath := isolateConfig(t)
	writeConfigFile(t, cfgPath, "127.0.0.1")

	for _, host := range []string{"0.0.0.0", "::", "0.0.0.0:43110", "192.168.1.10", "", "   "} {
		t.Run("host="+host, func(t *testing.T) {
			r := NewMgmtRouterWithAuth(MgmtAuthOptions{BindHost: host})

			w := getAPI(r, "/api/config", nil)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("bind host %q without password = %d, want 401", host, w.Code)
			}
		})
	}
}

// B4b: host classification must handle the spellings an operator actually
// passes, including a loopback address carrying a port. Shortening this rule
// once made "[::1]:43110" non-loopback; pin the cases either way.
func TestIsLoopback_RecognizesSpellings(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.1:43110", true},
		{"localhost", true},
		{"localhost:43110", true},
		{"::1", true},
		{"[::1]", true},
		{"[::1]:43110", true},
		{"LOCALHOST", true},
		{"", false},
		{"   ", false},
		{"0.0.0.0", false},
		{"::", false},
		{"0.0.0.0:43110", false},
		{"192.168.1.10", false},
		{"example.com:43110", false},
	}
	for _, tc := range cases {
		if got := IsLoopback(tc.host); got != tc.want {
			t.Errorf("IsLoopback(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

// B5: health probes stay reachable so an exposed-but-misconfigured listener is
// still diagnosable without credentials.
func TestMgmtAuth_HealthProbeStaysReachable(t *testing.T) {
	isolateConfig(t)
	r := NewMgmtRouterWithAuth(MgmtAuthOptions{BindHost: "0.0.0.0", Password: testPassword})

	w := getAPI(r, "/healthz", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /healthz = %d, want 200 without credentials", w.Code)
	}
}

// B6: the reported posture must match the posture actually enforced. The config
// file declares loopback while the listener is exposed, so a handler reading
// the config file reports authRequired:false and fails here.
func TestMgmtAuth_WebUIInfoReportsEnforcedPosture(t *testing.T) {
	cfgPath := isolateConfig(t)
	writeConfigFile(t, cfgPath, "127.0.0.1")

	exposed := NewMgmtRouterWithAuth(MgmtAuthOptions{BindHost: "0.0.0.0", Password: testPassword})
	if got := authRequired(t, exposed, map[string]string{"Authorization": basicAuthHeader("admin", testPassword)}); !got {
		t.Fatal("exposed listener reported authRequired=false, want true")
	}

	loopback := NewMgmtRouterWithAuth(MgmtAuthOptions{BindHost: "127.0.0.1"})
	if got := authRequired(t, loopback, nil); got {
		t.Fatal("loopback listener reported authRequired=true, want false")
	}
}

// B7: the startup guard refuses to expose the API without credentials, and
// names the explicit opt-in instead of silently inventing one.
func TestValidateBindAuth_RefusesExposedWithoutPassword(t *testing.T) {
	// "" is in this list because `webui start --host ""` binds every interface:
	// the guard must not read it as "unspecified, therefore local".
	for _, host := range []string{"0.0.0.0", "::", "192.168.1.10", "", "   "} {
		err := ValidateBindAuth(MgmtAuthOptions{BindHost: host})
		if err == nil {
			t.Fatalf("exposing bind host %q without a password must be refused", host)
		}
		if !strings.Contains(err.Error(), "--generate-password") {
			t.Fatalf("error must point at --generate-password, got %q", err.Error())
		}
	}
}

// B7b: the guard reads the resolved password, so a password from any supported
// source satisfies it and an empty credential never does.
func TestValidateBindAuth_RejectsEmptyCredential(t *testing.T) {
	if err := ValidateBindAuth(MgmtAuthOptions{BindHost: "0.0.0.0", Password: "   "}); err == nil {
		t.Fatal("a blank password must not satisfy the guard")
	}
}

func TestValidateBindAuth_AllowsExplicitChoices(t *testing.T) {
	cases := []struct {
		name string
		opts MgmtAuthOptions
	}{
		{"loopback", MgmtAuthOptions{BindHost: "127.0.0.1"}},
		{"password provided", MgmtAuthOptions{BindHost: "0.0.0.0", Password: testPassword}},
		{"generate requested", MgmtAuthOptions{BindHost: "0.0.0.0", GeneratePassword: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateBindAuth(tc.opts); err != nil {
				t.Fatalf("expected start to be allowed, got %v", err)
			}
		})
	}
}

// B8: the announced password must be the one that actually authenticates, and
// the daemon child — which resolves the same inputs in its own process — must
// agree. A stale PI_SWITCH_WEBUI_PASSWORD sits in the environment to prove that
// asking for a generated credential overrides it instead of leaving the
// operator with a password that does not work.
func TestResolveAuthOptions_AnnouncedPasswordIsTheEnforcedOne(t *testing.T) {
	isolateConfig(t)
	t.Setenv("PI_SWITCH_WEBUI_PASSWORD", "stale-env-password")

	var announced []string
	opts, err := ResolveAuthOptions("0.0.0.0", true, func(msg string) { announced = append(announced, msg) })
	if err != nil {
		t.Fatalf("ResolveAuthOptions: %v", err)
	}
	if len(announced) != 1 {
		t.Fatalf("generated password must be announced exactly once, got %d messages", len(announced))
	}
	if strings.Contains(announced[0], "stale-env-password") {
		t.Fatalf("announced the stale environment password: %q", announced[0])
	}

	// The announced credential is the one the listener accepts.
	r := NewMgmtRouterWithAuth(opts)
	served := getAPI(r, "/api/config", map[string]string{"Authorization": basicAuthHeader("admin", opts.Password)})
	if served.Code != http.StatusOK {
		t.Fatalf("announced password rejected with %d, want 200", served.Code)
	}
	stale := getAPI(r, "/api/config", map[string]string{"Authorization": basicAuthHeader("admin", "stale-env-password")})
	if stale.Code != http.StatusUnauthorized {
		t.Fatalf("stale environment password accepted with %d, want 401", stale.Code)
	}

	// The daemon child resolves independently and must land on the same secret.
	child, err := ResolveAuthOptions("0.0.0.0", false, nil)
	if err != nil {
		t.Fatalf("child ResolveAuthOptions: %v", err)
	}
	if child.Password != opts.Password {
		t.Fatal("child enforced a different password than the parent announced")
	}
}

// B9: a credential supplied through the documented environment variable is
// honored while nothing has been persisted yet, and it satisfies the guard.
func TestResolveAuthOptions_EnvPasswordIsHonored(t *testing.T) {
	isolateConfig(t)
	t.Setenv("PI_SWITCH_WEBUI_PASSWORD", "env-only-password")

	opts, err := ResolveAuthOptions("0.0.0.0", false, nil)
	if err != nil {
		t.Fatalf("ResolveAuthOptions: %v", err)
	}
	if opts.Password != "env-only-password" {
		t.Fatalf("password = %q, want the environment value", opts.Password)
	}
	if err := ValidateBindAuth(opts); err != nil {
		t.Fatalf("environment password must satisfy the startup guard: %v", err)
	}
}

// B10: the generated credential is persisted as a private file, which is what
// lets a restarted daemon reuse it instead of minting a new one.
func TestGenerateAndStorePassword_WritesPrivateFile(t *testing.T) {
	isolateConfig(t)

	first, err := GenerateAndStorePassword()
	if err != nil {
		t.Fatalf("GenerateAndStorePassword: %v", err)
	}
	if len(first) < 16 {
		t.Fatalf("generated password %q is too short to be a credential", first)
	}
	info, err := os.Stat(webUIPasswordPath())
	if err != nil {
		t.Fatalf("password file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("password file mode = %o, want 600", perm)
	}
	if stored := StoredWebUIPassword(); stored != first {
		t.Fatalf("persisted %q does not match the generated %q", stored, first)
	}

	second, err := GenerateAndStorePassword()
	if err != nil {
		t.Fatalf("second GenerateAndStorePassword: %v", err)
	}
	if second == first {
		t.Fatal("regenerating must not reuse the previous credential")
	}
}
