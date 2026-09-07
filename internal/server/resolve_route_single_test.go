package server

import (
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func ptr(s string) *string { return &s }

func TestResolveRoute_SingleCandidate(t *testing.T) {
	// Setup: two suppliers both exposing gpt-4o, current = supplier-a
	current := "supplier-a"
	cfg := config.PiSwitchConfig{
		Version: 2,
		Current: &current,
		Profiles: map[string]config.ProviderProfile{
			"supplier-a": {
				API:           "openai-completions",
				ResponsesMode: "auto",
				BaseURL:       "https://a.example.com/v1",
				APIKey:        "sk-a",
				Models:        []config.ModelEntry{{ID: "gpt-4o"}},
				ExposedModels: []string{"gpt-4o"},
			},
			"supplier-b": {
				API:           "openai-completions",
				ResponsesMode: "auto",
				BaseURL:       "https://b.example.com/v1",
				APIKey:        "sk-b",
				Models:        []config.ModelEntry{{ID: "gpt-4o"}},
				ExposedModels: []string{"gpt-4o"},
			},
		},
		Settings: config.Settings{
			ProviderPrefix:     "pi-switch",
			WriteMode:          "gateway",
			GatewayAPI:         "openai-completions",
			ConversationSource: "sessionScan",
			Proxy: struct {
				Host                          string                       `json:"host"`
				Port                          int                          `json:"port"`
				UserAgent                     *string                      `json:"userAgent,omitempty"`
				CircuitBreaker                config.CircuitBreakerSettings `json:"circuitBreaker"`
				RequestRetry                  *int                         `json:"requestRetry,omitempty"`
				MaxRetryCredentials           int                          `json:"maxRetryCredentials,omitempty"`
				MaxRetryInterval              *int                         `json:"maxRetryInterval,omitempty"`
				DisableCooling                *bool                        `json:"disableCooling,omitempty"`
				TransientErrorCooldownSeconds *int                         `json:"transientErrorCooldownSeconds,omitempty"`
				RequestScopedErrors           []config.RequestScopedError   `json:"requestScopedErrors,omitempty"`
			}{Host: "127.0.0.1", Port: 43112, CircuitBreaker: config.CircuitBreakerSettings{Enabled: true, FailureThreshold: 3, CooldownSeconds: 60}},
			Web: struct {
				Host string `json:"host"`
				Port int    `json:"port"`
			}{Host: "127.0.0.1", Port: 43110},
		},
	}

	// Case 1: bare model globally unique -> now ambiguous when two suppliers expose same bare
	cands, real, pinned := resolveRoute(cfg, "gpt-4o")
	if pinned != "ambiguous" {
		t.Fatalf("bare model ambiguous: got candidates %v pinned %q want ambiguous", cands, pinned)
	}
	_ = real

	// Case 2: supplier/model -> only that supplier, no failover append
	cands, real, pinned = resolveRoute(cfg, "supplier-a/gpt-4o")
	if len(cands) != 1 || cands[0] != "supplier-a" {
		t.Fatalf("supplier/model: got %v, want [supplier-a]", cands)
	}
	if real != "gpt-4o" {
		t.Fatalf("supplier/model real=%q want gpt-4o", real)
	}

	// Case 3: bare model when current does not expose but other does -> should hit supplier-b (global scan)
	current2 := "supplier-a"
	cfg2 := cfg
	cfg2.Current = &current2
	// make supplier-a not expose gpt-4o
	pa := cfg2.Profiles["supplier-a"]
	pa.ExposedModels = []string{"other-model"}
	cfg2.Profiles["supplier-a"] = pa
	cands, _, _ = resolveRoute(cfg2, "gpt-4o")
	if len(cands) != 1 || cands[0] != "supplier-b" {
		t.Fatalf("bare model global: got %v, want [supplier-b]", cands)
	}

	// Case 4: supplier/channel/model pin still works (single candidate + pinned)
	chName := "main"
	cfg3 := config.PiSwitchConfig{
		Version: 2,
		Current: &current,
		Profiles: map[string]config.ProviderProfile{
			"supplier-a": {
				API:           "openai-completions",
				ResponsesMode: "auto",
				Upstreams: []config.Upstream{
					{Name: &chName, BaseURL: "https://a.example.com/v1", APIKey: "sk-a", Models: []config.ModelEntry{{ID: "gpt-4o"}}, ExposedModels: []string{"gpt-4o"}},
				},
			},
		},
		Settings: cfg.Settings,
	}
	cands, real, pinned = resolveRoute(cfg3, "supplier-a/main/gpt-4o")
	if len(cands) != 1 || cands[0] != "supplier-a" || real != "gpt-4o" || pinned != "main" {
		t.Fatalf("supplier/channel/model: got %v %q %q, want [supplier-a] gpt-4o main", cands, real, pinned)
	}
}
