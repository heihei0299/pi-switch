package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func TestConfigMutationPreservesMissingProfilePriority(t *testing.T) {
	isolateConfig(t)
	if err := os.WriteFile(configPath(), []byte(configJSON), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, body string }{
		{"/api/profiles/missing/models", `{"models":[{"id":""}]}`},
		{"/api/profiles/missing/spoof", `{"spoof":"invalid"}`},
		{"/api/profiles/missing/expose", `{"modelIds":[]}`},
	} {
		w := httptest.NewRecorder()
		NewMgmtRouter().ServeHTTP(w, httptest.NewRequest(http.MethodPut, tc.path, strings.NewReader(tc.body)))
		if w.Code != 404 {
			t.Errorf("%s HTTP %d, want 404: %s", tc.path, w.Code, w.Body.String())
		}
	}
}

func TestPutSpoofUsecasePreservesValidationAndNoWriteBehavior(t *testing.T) {
	isolateConfig(t)
	path := configPath()
	if err := os.WriteFile(path, []byte(configJSON), 0600); err != nil {
		t.Fatal(err)
	}
	r := NewMgmtRouter()
	put := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/profiles/p1/spoof", strings.NewReader(body)))
		return w
	}
	if w := put(`{"spoof":"codex"}`); w.Code != 200 {
		t.Fatalf("set spoof HTTP %d: %s", w.Code, w.Body.String())
	}
	cfg, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Profiles["p1"].UserAgent; got == nil || *got != "codex" {
		t.Fatalf("userAgent = %v, want codex", got)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if w := put(`{"spoof":"invalid"}`); w.Code != 400 || !strings.Contains(w.Body.String(), "invalid spoof") {
		t.Fatalf("invalid spoof HTTP %d: %s", w.Code, w.Body.String())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("invalid spoof changed config bytes")
	}
	if w := put(`{"spoof":""}`); w.Code != 200 {
		t.Fatalf("clear spoof HTTP %d: %s", w.Code, w.Body.String())
	}
	cfg, _, err = config.LoadConfigAtPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Profiles["p1"].UserAgent != nil {
		t.Fatalf("empty spoof was not cleared: %v", cfg.Profiles["p1"].UserAgent)
	}
}
