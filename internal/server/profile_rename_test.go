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

func renameFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	config := `{"version":2,"current":"alpha","profiles":{
"alpha":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://alpha","apiKey":"alpha-key","upstreams":[{"name":"main","api":"openai-completions","baseUrl":"http://alpha","apiKey":"alpha-key","models":[{"id":"alpha-model"}],"exposedModels":["alpha-model"]}]},
"beta":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://beta","apiKey":"beta-key","upstreams":[{"name":"main","api":"openai-completions","baseUrl":"http://beta","apiKey":"beta-key","models":[{"id":"beta-model"}],"exposedModels":["beta-model"]}]}},"settings":{}}`
	if err := os.WriteFile(path, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", path)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	return path
}

func renameRequest(t *testing.T, r http.Handler, name, renameFrom string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"profile":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://renamed","apiKey":"renamed-key","upstreams":[{"name":"main","api":"openai-completions","baseUrl":"http://renamed","apiKey":"renamed-key","models":[{"id":"renamed-model"}],"exposedModels":["renamed-model"]}]},"renameFrom":"` + renameFrom + `"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/profiles/"+name, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestProfileRename_ExistingTargetIsAtomic(t *testing.T) {
	path := renameFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	w := renameRequest(t, NewMgmtRouter(), "beta", "alpha")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("rename to existing target = %d (%s), want 400", w.Code, w.Body.String())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("failed rename changed config:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestProfileRename_UpdatesCurrent(t *testing.T) {
	path := renameFixture(t)
	w := renameRequest(t, NewMgmtRouter(), "gamma", "alpha")
	if w.Code != http.StatusOK {
		t.Fatalf("rename active profile = %d (%s), want 200", w.Code, w.Body.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Current  *string `json:"current"`
		Profiles map[string]json.RawMessage `json:"profiles"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Current == nil || *cfg.Current != "gamma" {
		t.Fatalf("current = %v, want gamma", cfg.Current)
	}
	if _, ok := cfg.Profiles["alpha"]; ok {
		t.Fatal("old profile still exists")
	}
	if _, ok := cfg.Profiles["gamma"]; !ok {
		t.Fatal("renamed profile missing")
	}
}
