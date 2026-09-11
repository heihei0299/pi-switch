package profile

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func isolate(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PI_SWITCH_CONFIG", path)
	return path
}

func provider() config.ProviderProfile {
	return config.ProviderProfile{
		API:           "openai-completions",
		ResponsesMode: "auto",
		BaseURL:       "https://example.test/v1",
		Upstreams: []config.Upstream{{
			Name:    strptr("main"),
			API:     "openai-completions",
			BaseURL: "https://example.test/v1",
			Models:  []config.ModelEntry{{ID: "m1", ContextWindow: 100, MaxTokens: 10}},
		}},
	}
}

func TestCreateProfilePersistsAndRejectsDuplicates(t *testing.T) {
	path := isolate(t)
	if err := CreateProfile("p", provider()); err != nil {
		t.Fatalf("create: %v", err)
	}
	cfg, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Profiles["p"]; !ok {
		t.Fatalf("profile not persisted: %+v", cfg.Profiles)
	}
	if err := CreateProfile("p", provider()); err == nil {
		t.Fatal("duplicate profile accepted")
	}
}

func TestDuplicateProfileErrorKinds(t *testing.T) {
	isolate(t)
	if err := CreateProfile("p", provider()); err != nil {
		t.Fatal(err)
	}
	if err := DuplicateProfile("p", ""); !errors.Is(err, ErrTargetNameRequired) {
		t.Fatalf("empty target = %v, want ErrTargetNameRequired", err)
	}
	if err := DuplicateProfile("missing", "q"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("missing source = %v, want ErrProfileNotFound", err)
	}
	if err := DuplicateProfile("p", "q"); err != nil {
		t.Fatalf("duplicate: %v", err)
	}
}

func TestSetExposedModelsValidatesChannelAndPool(t *testing.T) {
	isolate(t)
	if err := CreateProfile("p", provider()); err != nil {
		t.Fatal(err)
	}
	if err := SetExposedModels("p", "nope", []string{"m1"}); !errors.Is(err, ErrUnknownChannel) {
		t.Fatalf("unknown channel = %v, want ErrUnknownChannel", err)
	}
	if err := SetExposedModels("p", "main", []string{"not-in-pool"}); err == nil {
		t.Fatal("exposing a model outside the channel pool was accepted")
	}
	if err := SetExposedModels("p", "main", []string{"m1"}); err != nil {
		t.Fatalf("expose: %v", err)
	}
}

func TestEnsureMutationChannelUpgradesLegacyUnnamedUpstream(t *testing.T) {
	prof := config.ProviderProfile{
		API:     "openai-completions",
		BaseURL: "https://legacy.test/v1",
		APIKey:  "k",
		Upstreams: []config.Upstream{{
			API:     "openai-completions",
			BaseURL: "https://legacy.test/v1",
			APIKey:  "k",
		}},
	}
	idx := EnsureMutationChannel(&prof, "main")
	if idx != 0 || prof.ChannelName(0) != "main" {
		t.Fatalf("legacy upgrade failed: idx=%d prof=%+v", idx, prof.Upstreams)
	}
	if got := EnsureMutationChannel(&prof, "other"); got != -1 {
		t.Fatalf("unknown channel = %d, want -1", got)
	}
}

func strptr(s string) *string { return &s }
