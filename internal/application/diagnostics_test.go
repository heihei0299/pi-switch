package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func TestDiagnoseConfigWarnsForUnknownModelsDevProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	unknown := "unknown-provider"
	cfg := config.DefaultConfig()
	cfg.Profiles = map[string]config.ProviderProfile{
		"provider": {API: "openai-completions", BaseURL: "https://example.test", ModelsDevProvider: &unknown},
	}
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}

	_, issues := DiagnoseConfig(path)
	for _, issue := range issues {
		if issue.Path == "profiles.provider.modelsDevProvider" && issue.Level == "warning" && issue.Message == "modelsDevProvider unknown: unknown-provider" {
			return
		}
	}
	t.Fatalf("missing unknown models.dev provider warning: %#v", issues)
}

func TestDiagnoseConfigUnreadableAndEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"profiles":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	checks, issues := DiagnoseConfig(path)
	if len(checks) != 2 || !checks[0].OK || checks[1].OK || issues == nil || len(issues) != 0 {
		t.Fatalf("empty diagnostics = %#v / %#v", checks, issues)
	}
	if err := os.WriteFile(path, []byte(`{`), 0600); err != nil {
		t.Fatal(err)
	}
	checks, issues = DiagnoseConfig(path)
	if len(checks) != 1 || checks[0].OK || len(issues) != 1 || issues[0].Path != "config" {
		t.Fatalf("invalid diagnostics = %#v / %#v", checks, issues)
	}
}

func TestDiagnoseConfigHidesCredentialsInValidationMessages(t *testing.T) {
	path, cfg := managementFixture(t)
	prof := cfg.Profiles["supplier"]
	prof.APIKey = "fake-\"quoted\nkey"
	prof.RequestScopedErrors = []config.RequestScopedError{{Status: 429, Action: prof.APIKey}}
	endpoint := prof.Upstreams[0].BaseURL
	prof.ModelsDevProvider = &endpoint
	cfg.Profiles["supplier"] = prof
	cfg.Settings.Proxy.RequestScopedErrors = []config.RequestScopedError{{Status: 429, Action: "fake-settings-secret"}}
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, issues := DiagnoseConfig(path)
	providerRetry, settingsRetry := false, false
	for _, issue := range issues {
		providerRetry = providerRetry || issue.Path == "profiles.supplier.retry"
		settingsRetry = settingsRetry || issue.Path == "settings.proxy.retry"
		for _, secret := range []string{"quoted", "fake-password", "fake-query-key", "fake-settings-secret"} {
			if strings.Contains(issue.Message, secret) {
				t.Fatalf("diagnostic leaked %s: %+v", secret, issue)
			}
		}
	}
	if !providerRetry || !settingsRetry {
		t.Fatalf("lost retry field diagnostics: %+v", issues)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("diagnostics modified configuration")
	}
}
