package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// known-gaps/02(c): GET /api/gateway/health answered has_models_file: true as a
// literal, never checked against the file it names. The frontend decodes that field
// as a required boolean (webui/src/apiSchema.ts), so it reads as "already
// published" whether or not anything was published.
//
// PI_SWITCH_MODELS is set explicitly here: isolateConfig does not isolate it, so
// without this the test would read the developer's real ~/.pi/agent/models.json and
// pass or fail depending on their machine.
func TestGatewayHealth_ModelsFileFlagReflectsReality(t *testing.T) {
	isolateConfig(t)
	dir := t.TempDir()
	models := filepath.Join(dir, "models.json")
	t.Setenv("PI_SWITCH_MODELS", models)

	r := NewMgmtRouter()

	// Nothing published yet.
	w := callMgmt(r, http.MethodGet, "/api/gateway/health", "")
	if w.Code != http.StatusOK {
		t.Fatalf("gateway health = %d (%s), want 200", w.Code, w.Body.String())
	}
	if has := decodeHealth(t, w).HasModelsFile; has {
		t.Fatalf("has_models_file = true although %s does not exist: %s", models, w.Body.String())
	}

	// After a publish, the same field must flip.
	if err := os.WriteFile(models, []byte(`{"providers":{"pi-switch-chat":{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","models":[]}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	w2 := callMgmt(r, http.MethodGet, "/api/gateway/health", "")
	if has := decodeHealth(t, w2).HasModelsFile; !has {
		t.Fatalf("has_models_file = false although %s exists: %s", models, w2.Body.String())
	}
}

// The neighbouring real field must keep working: upstreams_total counts profiles.
func TestGatewayHealth_UpstreamsTotalStillCountsProfiles(t *testing.T) {
	cfgPath := isolateConfig(t)
	t.Setenv("PI_SWITCH_MODELS", filepath.Join(t.TempDir(), "models.json"))
	two := `{"version":2,"profiles":{"a":{"api":"openai-responses","baseUrl":"http://a","apiKey":"k"},"b":{"api":"openai-responses","baseUrl":"http://b","apiKey":"k"}},"settings":{}}`
	if err := os.WriteFile(cfgPath, []byte(two), 0644); err != nil {
		t.Fatal(err)
	}

	w := callMgmt(NewMgmtRouter(), http.MethodGet, "/api/gateway/health", "")
	if got := decodeHealth(t, w).UpstreamsTotal; got != 2 {
		t.Fatalf("upstreams_total = %d, want 2 (%s)", got, w.Body.String())
	}
}

type healthBody struct {
	HasModelsFile  bool `json:"has_models_file"`
	UpstreamsTotal int  `json:"upstreams_total"`
}

func decodeHealth(t *testing.T, w *httptest.ResponseRecorder) healthBody {
	t.Helper()
	var body healthBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	return body
}
