package providers_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/heihei0299/pi-switch/internal/bootstrap"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/domain/provider"
	providerusecase "github.com/heihei0299/pi-switch/internal/usecase/providers"
)

func isolatedProviderService(t *testing.T) (*providerusecase.Service, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PI_SWITCH_CONFIG", path)
	return bootstrap.NewProviderService(path), path
}

func validProvider() provider.Profile {
	main := "main"
	return provider.Profile{
		API: "openai-completions", ResponsesMode: "auto", BaseURL: "https://example.test/v1", APIKey: "fake-key",
		Upstreams: []provider.Upstream{{
			Name: &main, API: "openai-completions", BaseURL: "https://example.test/v1", APIKey: "fake-key",
			Models: []provider.ModelEntry{{ID: "m1", ContextWindow: 100, MaxTokens: 10}},
		}},
	}
}

func TestLegacyCreatePersistsAndRejectsDuplicates(t *testing.T) {
	service, path := isolatedProviderService(t)
	if result, err := service.CreateLegacy(context.Background(), " p ", validProvider()); err != nil || result.Name != "p" {
		t.Fatalf("create = %+v, %v", result, err)
	}
	cfg, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Profiles["p"]; !ok {
		t.Fatalf("profile not persisted: %+v", cfg.Profiles)
	}
	if _, err := service.CreateLegacy(context.Background(), "p", validProvider()); !errors.Is(err, providerusecase.ErrProfileExists) {
		t.Fatalf("duplicate create = %v, want ErrProfileExists", err)
	}
	if _, err := os.Stat(path + ".backups"); !os.IsNotExist(err) {
		t.Fatalf("legacy create made backups: %v", err)
	}
}

func TestLegacyCreateUsesFullValidationBeforeWriting(t *testing.T) {
	service, path := isolatedProviderService(t)
	flat := func(api string) provider.Profile {
		return provider.Profile{API: api, ResponsesMode: "auto", BaseURL: "https://example.test/v1", APIKey: "fake-key"}
	}
	for _, tc := range []struct{ api, message string }{
		{"google-generative-ai", "not currently proxy-supported"},
		{"unknown-api", "unsupported api"},
	} {
		if _, err := service.CreateLegacy(context.Background(), "bad-"+tc.api, flat(tc.api)); err == nil || !strings.Contains(err.Error(), tc.message) {
			t.Fatalf("create %s = %v, want %q", tc.api, err, tc.message)
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("rejected create touched config: %v", err)
	}
}

func TestLegacyDuplicatePreservesCapabilityAndAdvisoryRules(t *testing.T) {
	cases := []struct {
		name, profile, wantError string
	}{
		{"flat unproxyable", `{"api":"google-generative-ai","baseUrl":"https://example.test/v1","apiKey":"k"}`, "not currently proxy-supported"},
		{"flat unknown", `{"api":"unknown-api","baseUrl":"https://example.test/v1","apiKey":"k"}`, "unsupported api"},
		{"channel unproxyable", `{"api":"openai-completions","baseUrl":"https://example.test/v1","apiKey":"k","upstreams":[{"name":"main","api":"google-generative-ai","baseUrl":"https://example.test/v1","apiKey":"k","models":[{"id":"m1"}]}]}`, "upstreams[0]"},
		{"shape remains advisory", `{"api":"openai-completions","baseUrl":"ftp://example.test","apiKey":"k"}`, ""},
		{"top-level mode checked", `{"api":"openai-completions","responsesMode":"passthrough","baseUrl":"https://example.test/v1","apiKey":"k","upstreams":[{"name":"main","api":"openai-responses","responsesMode":"passthrough","baseUrl":"https://example.test/v1","apiKey":"k","models":[{"id":"m1"}]}]}`, "passthrough"},
		{"channel override allowed", `{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://example.test/v1","apiKey":"k","upstreams":[{"name":"main","api":"openai-responses","responsesMode":"passthrough","baseUrl":"https://example.test/v1","apiKey":"k","models":[{"id":"m1"}]}]}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service, path := isolatedProviderService(t)
			original := []byte(`{"version":2,"profiles":{"legacy":` + tc.profile + `}}`)
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			_, err := service.DuplicateLegacy(context.Background(), "legacy", "copy")
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("duplicate = %v", err)
				}
				cfg, _, loadErr := config.LoadConfigAtPath(path)
				if loadErr != nil {
					t.Fatal(loadErr)
				}
				if _, ok := cfg.Profiles["copy"]; !ok {
					t.Fatal("copy was not persisted")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("duplicate = %v, want error containing %q", err, tc.wantError)
			}
			after, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(after) != string(original) {
				t.Fatalf("rejected duplicate changed config: %s", after)
			}
		})
	}
}

func TestLegacyDuplicateErrorKindsAndExposeValidation(t *testing.T) {
	service, _ := isolatedProviderService(t)
	if _, err := service.CreateLegacy(context.Background(), "p", validProvider()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DuplicateLegacy(context.Background(), "p", ""); !errors.Is(err, providerusecase.ErrTargetNameRequired) {
		t.Fatalf("empty target = %v", err)
	}
	if _, err := service.DuplicateLegacy(context.Background(), "missing", "q"); !errors.Is(err, providerusecase.ErrProfileNotFound) {
		t.Fatalf("missing source = %v", err)
	}
	if _, err := service.DuplicateLegacy(context.Background(), "p", "q"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DuplicateLegacy(context.Background(), "q", "q"); !errors.Is(err, providerusecase.ErrProfileExists) {
		t.Fatalf("existing target = %v", err)
	}
	if err := service.SetExposedModelsLegacy(context.Background(), "p", "missing", []string{"m1"}); !errors.Is(err, providerusecase.ErrUnknownChannel) {
		t.Fatalf("unknown channel = %v", err)
	}
	if err := service.SetExposedModelsLegacy(context.Background(), "p", "main", []string{"missing"}); err == nil {
		t.Fatal("unknown model was exposed")
	}
	if err := service.SetExposedModelsLegacy(context.Background(), "p", "main", []string{"m1"}); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentLegacyCreatesPreserveProfiles(t *testing.T) {
	_, path := isolatedProviderService(t)
	const count = 12
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := bootstrap.NewProviderService(path).CreateLegacy(context.Background(), fmt.Sprintf("p%d", i), validProvider())
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	cfg, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		t.Fatal(err)
	}
	want := count + len(config.DefaultConfig().Profiles)
	if len(cfg.Profiles) != want {
		t.Fatalf("persisted profiles = %d, want %d", len(cfg.Profiles), want)
	}
	if _, err := os.Stat(path + ".backups"); !os.IsNotExist(err) {
		t.Fatalf("concurrent legacy creates made backups: %v", err)
	}
}
