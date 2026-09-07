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
				Upstreams:     []config.Upstream{{Name: ptr("main"), API: "openai-completions", BaseURL: "https://a.example.com/v1", APIKey: "sk-a", Models: []config.ModelEntry{{ID: "gpt-4o"}}, ExposedModels: []string{"gpt-4o"}}},
			},
			"supplier-b": {
				API:           "openai-completions",
				ResponsesMode: "auto",
				BaseURL:       "https://b.example.com/v1",
				APIKey:        "sk-b",
				Upstreams:     []config.Upstream{{Name: ptr("main"), API: "openai-completions", BaseURL: "https://b.example.com/v1", APIKey: "sk-b", Models: []config.ModelEntry{{ID: "gpt-4o"}}, ExposedModels: []string{"gpt-4o"}}},
			},
		},
	}

	// Case 1: bare model globally unique -> now ambiguous when two suppliers expose same bare
	cands, real, pinned := resolveRoute(cfg, "gpt-4o")
	if pinned != "ambiguous" {
		t.Fatalf("bare model ambiguous: got candidates %v pinned %q want ambiguous", cands, pinned)
	}
	_ = real

	// Case 2: slash-prefixed ids are no longer routable.
	cands, real, pinned = resolveRoute(cfg, "supplier-a/gpt-4o")
	if len(cands) != 0 || real != "supplier-a/gpt-4o" || pinned != "" {
		t.Fatalf("supplier/model: got %v %q %q, want no route", cands, real, pinned)
	}

	// Case 3: bare model when current does not expose but other does -> should hit supplier-b (global scan)
	current2 := "supplier-a"
	cfg2 := cfg
	cfg2.Current = &current2
	// make supplier-a not expose gpt-4o
	pa := cfg2.Profiles["supplier-a"]
	pa.Upstreams[0].ExposedModels = []string{"other-model"}
	cfg2.Profiles["supplier-a"] = pa
	cands, _, _ = resolveRoute(cfg2, "gpt-4o")
	if len(cands) != 1 || cands[0] != "supplier-b" {
		t.Fatalf("bare model global: got %v, want [supplier-b]", cands)
	}

	// Case 4: supplier/channel/model is also rejected by resolveRoute.
	chName := "main"
	cfg3 := config.PiSwitchConfig{
		Version: 2,
		Current: &current,
		Profiles: map[string]config.ProviderProfile{
			"supplier-a": {
				API:           "openai-completions",
				ResponsesMode: "auto",
				Upstreams: []config.Upstream{
					{Name: &chName, API: "openai-completions", BaseURL: "https://a.example.com/v1", APIKey: "sk-a", Models: []config.ModelEntry{{ID: "gpt-4o"}}, ExposedModels: []string{"gpt-4o"}},
				},
			},
		},
	}
	cands, real, pinned = resolveRoute(cfg3, "supplier-a/main/gpt-4o")
	if len(cands) != 0 || real != "supplier-a/main/gpt-4o" || pinned != "" {
		t.Fatalf("supplier/channel/model: got %v %q %q, want no route", cands, real, pinned)
	}
}
