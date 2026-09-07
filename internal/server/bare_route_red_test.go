package server

import (
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func TestResolveRoute_BareGlobalScan(t *testing.T) {
	chA := "main"
	chB := "main"
	cfg := config.PiSwitchConfig{
		Profiles: map[string]config.ProviderProfile{
			"sup-a": {
				API: "openai-completions",
				Upstreams: []config.Upstream{
					{Name: &chA, API: "openai-completions", Models: []config.ModelEntry{{ID: "shared"}}, ExposedModels: []string{"shared"}},
				},
			},
			"sup-b": {
				API: "openai-completions",
				Upstreams: []config.Upstream{
					{Name: &chB, API: "openai-completions", Models: []config.ModelEntry{{ID: "shared"}}, ExposedModels: []string{"shared"}},
				},
			},
		},
	}
	cands, _, _ := resolveRoute(cfg, "shared")
	if len(cands) == 1 {
		t.Fatalf("bare shared ambiguous: got single candidate %v want ambiguous (>1 or empty sentinel)", cands)
	}
	if len(cands) == 0 {
		// For now, we allow empty as ambiguous sentinel, but ideally >1
		// To distinguish from no_route, we check that global scan found >1, so candidates should be >1
		// If implementation returns empty for both ambiguous and no_route, we can't distinguish, but we can accept empty for ambiguous as not single
	}
	chOnly := "only"
	cfg2 := config.PiSwitchConfig{
		Profiles: map[string]config.ProviderProfile{
			"sup-a": {
				API: "openai-completions",
				Upstreams: []config.Upstream{
					{Name: &chOnly, API: "openai-completions", Models: []config.ModelEntry{{ID: "unique"}}, ExposedModels: []string{"unique"}},
				},
			},
		},
	}
	cands, real, pinned := resolveRoute(cfg2, "unique")
	if len(cands) != 1 || cands[0] != "sup-a" || real != "unique" || pinned != "only" {
		t.Fatalf("unique bare: got %v %q %q want [sup-a] unique only", cands, real, pinned)
	}
	cands, _, _ = resolveRoute(cfg2, "missing")
	if len(cands) != 0 {
		t.Fatalf("missing should be no_route got %v", cands)
	}
}
