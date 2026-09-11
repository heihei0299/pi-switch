package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// ARCH-01: a corrupt config must fail the read endpoints explicitly instead of
// being served as a default/empty config.
func TestStrictConfig_CorruptFileFailsReadEndpoints(t *testing.T) {
	cfgPath := isolateConfig(t)
	if err := os.WriteFile(cfgPath, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	r := NewMgmtRouter()
	for _, path := range []string{"/api/config", "/api/state", "/api/profiles", "/api/settings"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("GET %s with corrupt config = %d, want 500 (body=%s)", path, w.Code, w.Body.String())
		}
	}
}

// A missing file is still the default config, so the read endpoints keep working
// before the first save.
func TestStrictConfig_MissingFileStillServesDefault(t *testing.T) {
	isolateConfig(t) // config file intentionally not created
	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/settings without config = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
}
