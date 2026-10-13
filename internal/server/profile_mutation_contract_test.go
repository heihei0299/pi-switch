package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/application"
	"github.com/heihei0299/pi-switch/internal/config"
)

const legacyProfileMutationConfig = `{"version":2,"current":"source","profiles":{"source":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://source.example/v1","apiKey":"fake-source-key","models":[{"id":"source-model","contextWindow":100,"maxTokens":10}]},"keep":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://keep.example/v1","apiKey":"fake-keep-key","models":[{"id":"keep-model","contextWindow":100,"maxTokens":10}]}},"settings":{}}`

const replacementProfile = `{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://replacement.example/v1","apiKey":"fake-replacement-key","models":[{"id":"replacement-model","contextWindow":100,"maxTokens":10}]}`

func writeProfileMutationFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(legacyProfileMutationConfig), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyHTTPProfilePutPreservesRenameAndWriteContract(t *testing.T) {
	path := isolateConfig(t)
	expectedPath := filepath.Join(filepath.Dir(path), "expected.json")
	writeProfileMutationFixture(t, path)
	writeProfileMutationFixture(t, expectedPath)

	var prof config.ProviderProfile
	if err := json.Unmarshal([]byte(replacementProfile), &prof); err != nil {
		t.Fatal(err)
	}
	if err := config.UpdateAtPath(expectedPath, func(cfg *config.PiSwitchConfig) error {
		delete(cfg.Profiles, "source")
		cfg.Profiles["renamed"] = prof
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	body := `{"profile":` + replacementProfile + `,"renameFrom":"source"}`
	w := httptest.NewRecorder()
	NewMgmtRouter().ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/profiles/renamed", strings.NewReader(body)))
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"backup":null,"ok":true}` {
		t.Fatalf("PUT response = %d %s", w.Code, w.Body.String())
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("PUT config bytes differ from legacy mutation:\ngot:  %s\nwant: %s", got, want)
	}
	cfg, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Current == nil || *cfg.Current != "source" {
		t.Fatalf("legacy PUT rename changed current from source: %v", cfg.Current)
	}
	if _, err := os.Stat(path + ".backups"); !os.IsNotExist(err) {
		t.Fatalf("legacy PUT created backups, stat error = %v", err)
	}
}

func TestLegacyHTTPProfileDeletePreservesCurrentAndNoWriteContract(t *testing.T) {
	path := isolateConfig(t)
	expectedPath := filepath.Join(filepath.Dir(path), "expected.json")
	writeProfileMutationFixture(t, path)
	writeProfileMutationFixture(t, expectedPath)
	if err := config.UpdateAtPath(expectedPath, func(cfg *config.PiSwitchConfig) error {
		delete(cfg.Profiles, "source")
		cfg.Current = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	NewMgmtRouter().ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/profiles/source", nil))
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"backup":null,"ok":true}` {
		t.Fatalf("DELETE response = %d %s", w.Code, w.Body.String())
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("DELETE config bytes differ from legacy mutation:\ngot:  %s\nwant: %s", got, want)
	}

	before := append([]byte(nil), got...)
	missing := httptest.NewRecorder()
	NewMgmtRouter().ServeHTTP(missing, httptest.NewRequest(http.MethodDelete, "/api/profiles/missing", nil))
	if missing.Code != http.StatusNotFound || strings.TrimSpace(missing.Body.String()) != `{"error":"not found"}` {
		t.Fatalf("missing DELETE response = %d %s", missing.Code, missing.Body.String())
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, before) {
		t.Fatal("missing DELETE changed config bytes")
	}
	if _, err := os.Stat(path + ".backups"); !os.IsNotExist(err) {
		t.Fatalf("legacy DELETE created backups, stat error = %v", err)
	}
}

func TestLegacyHTTPProfilePutValidationPrecedesConfigIO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-parent", "config.json")
	t.Setenv("PI_SWITCH_CONFIG", path)
	w := httptest.NewRecorder()
	NewMgmtRouter().ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/profiles/new", strings.NewReader(`{"profile":{"api":"invalid-api"}}`)))
	if w.Code != http.StatusBadRequest || strings.Contains(w.Body.String(), "failed to save config") {
		t.Fatalf("invalid profile did not win over config IO failure: %d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("validation created config directory, stat error = %v", err)
	}
}

func TestHTTPAndNativeProviderMutationsDoNotLoseConcurrentUpdates(t *testing.T) {
	path := isolateConfig(t)
	writeProfileMutationFixture(t, path)

	var native config.ProviderProfile
	if err := json.Unmarshal([]byte(replacementProfile), &native); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := application.Revision(native)

	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		if err := application.SaveProvider(path, "source", "native-renamed", native, application.Revision(cfg.Profiles["source"])); err != nil {
			results <- err
			return
		}
		results <- nil
	}()
	go func() {
		<-start
		body := `{"profile":` + replacementProfile + `,"renameFrom":"keep"}`
		w := httptest.NewRecorder()
		NewMgmtRouter().ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/profiles/http-renamed", strings.NewReader(body)))
		if w.Code != http.StatusOK {
			results <- fmt.Errorf("HTTP PUT response = %d %s", w.Code, w.Body.String())
			return
		}
		results <- nil
	}()
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}

	got, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if application.Revision(got.Profiles["native-renamed"]) != expected || application.Revision(got.Profiles["http-renamed"]) != expected {
		t.Fatalf("concurrent mutations lost a provider: %+v", got.Profiles)
	}
	if _, ok := got.Profiles["source"]; ok {
		t.Fatal("Native rename retained source profile")
	}
	if _, ok := got.Profiles["keep"]; ok {
		t.Fatal("HTTP rename retained source profile")
	}
}
