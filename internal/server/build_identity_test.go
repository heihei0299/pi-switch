package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBuildInfoExposesAuditableIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	old := struct{ version, time, commit, target, dirty string }{Version, BuildTime, BuildCommit, BuildTarget, BuildDirty}
	defer func() {
		Version, BuildTime, BuildCommit, BuildTarget, BuildDirty = old.version, old.time, old.commit, old.target, old.dirty
	}()
	Version, BuildTime, BuildCommit, BuildTarget, BuildDirty = "v-test", "2026-09-10T00:00:00Z", "abc123", "linux/amd64", "false"

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/buildInfo", nil)
	NewMgmtRouter().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("buildInfo status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"version": "v-test", "buildTime": "2026-09-10T00:00:00Z", "commit": "abc123", "target": "linux/amd64", "dirty": "false",
	} {
		if payload[key] != want {
			t.Fatalf("buildInfo[%q]=%v, want %q", key, payload[key], want)
		}
	}
}
