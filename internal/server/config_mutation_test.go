package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
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
