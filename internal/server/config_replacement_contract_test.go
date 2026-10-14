package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func TestPutConfigReplacesWholeDocumentWithoutBackup(t *testing.T) {
	path := isolateConfig(t)
	old := `{"version":2,"current":"old","profiles":{"old":{"api":"openai-completions","baseUrl":"https://old.example/v1","apiKey":"old-key"}},"settings":{"proxy":{"host":"127.0.0.2","port":43210}}}`
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}

	body := `{"version":2,"current":"new","profiles":{"new":{"api":"openai-completions","baseUrl":"https://new.example/v1","apiKey":"new-key"}},"settings":{}}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	NewMgmtRouter().ServeHTTP(w, req)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"ok":true}` {
		t.Fatalf("PUT /api/config = %d %s, want 200 {\"ok\":true}", w.Code, w.Body.String())
	}

	got, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Current == nil || *got.Current != "new" {
		t.Fatalf("current = %v, want new", got.Current)
	}
	if _, ok := got.Profiles["old"]; ok {
		t.Fatalf("whole-document replacement retained old profile: %+v", got.Profiles)
	}
	if _, ok := got.Profiles["new"]; !ok {
		t.Fatalf("whole-document replacement lost new profile: %+v", got.Profiles)
	}
	if got.Settings.Proxy.Host != "127.0.0.1" || got.Settings.Proxy.Port != 43112 {
		t.Fatalf("replacement did not apply defaults: %+v", got.Settings.Proxy)
	}
	if _, err := os.Stat(path + ".backups"); !os.IsNotExist(err) {
		t.Fatalf("legacy full-config replacement created backups: %v", err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("replaced config mode = %v, want 0600", err)
	}
}

func TestPutConfigInvalidReplacementPreservesBytesAndNoBackup(t *testing.T) {
	path := isolateConfig(t)
	before := []byte(`{"version":2,"current":"old","profiles":{"old":{"api":"openai-completions"}},"settings":{}}`)
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(`{"version":2,"profiles":{"broken":{"api":"openai-completions","responsesMode":"passthrough"}},"settings":{}}`))
	req.Header.Set("Content-Type", "application/json")
	NewMgmtRouter().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "passthrough") {
		t.Fatalf("invalid PUT /api/config = %d %s, want validation error", w.Code, w.Body.String())
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("invalid replacement changed config bytes:\nbefore=%s\nafter=%s", before, after)
	}
	if _, err := os.Stat(path + ".backups"); !os.IsNotExist(err) {
		t.Fatalf("invalid full-config replacement created backups: %v", err)
	}
}
