package profile

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/protocol"
)

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

func TestValidateProfileCoversAllThreeRuleClasses(t *testing.T) {
	if err := ValidateProfile(provider()); err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}

	mode := provider()
	mode.ResponsesMode = "passthrough"
	if err := ValidateProfile(mode); err == nil || !strings.Contains(err.Error(), "responsesMode") {
		t.Fatalf("responsesMode rule missing: %v", err)
	}

	shape := provider()
	shape.Upstreams[0].BaseURL = "ftp://example.test"
	if err := ValidateProfile(shape); err == nil || !strings.Contains(err.Error(), "upstreams baseUrl") {
		t.Fatalf("shape rule missing: %v", err)
	}

	tooManyRetries := config.MaxRequestRetry + 1
	retry := provider()
	retry.RequestRetry = &tooManyRetries
	if err := ValidateProfile(retry); err == nil || !strings.Contains(err.Error(), "requestRetry") {
		t.Fatalf("retry rule missing: %v", err)
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

func TestFetchChannelModelEntriesDoesNotUpgradeInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"data":[{"id":"one"}]}`)) }))
	defer server.Close()
	prof := config.ProviderProfile{API: "openai-completions", BaseURL: server.URL, APIKey: "fake-key", Upstreams: []config.Upstream{{}}}
	if _, _, _, err := FetchChannelModelEntries(prof, "main"); err != nil {
		t.Fatal(err)
	}
	if prof.Upstreams[0].Name != nil || prof.Upstreams[0].API != "" || prof.Upstreams[0].BaseURL != "" || prof.Upstreams[0].APIKey != "" {
		t.Fatal("read-only fetch upgraded caller's legacy channel")
	}
}

func TestDuplicateValidationRejectsUnproxyableCapability(t *testing.T) {
	prof := config.ProviderProfile{API: protocol.GoogleGenerativeAI, ResponsesMode: "auto"}
	if err := ValidateResolvedCapability(prof); err == nil || !strings.Contains(err.Error(), "not currently proxy-supported") {
		t.Fatalf("unproxyable capability error = %v", err)
	}
}
