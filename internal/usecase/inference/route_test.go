package inference

import (
	"reflect"
	"testing"

	configdomain "github.com/heihei0299/pi-switch/internal/domain/config"
	"github.com/heihei0299/pi-switch/internal/domain/provider"
)

func TestResolveRouteUsesGloballyUniqueExposure(t *testing.T) {
	current := "supplier-a"
	main := "main"
	other := "other"
	cfg := configdomain.PiSwitchConfig{
		Current: &current,
		Profiles: map[string]provider.Profile{
			"supplier-a": {Upstreams: []provider.Upstream{{Name: &main, ExposedModels: []string{"other-model"}}}},
			"supplier-b": {Upstreams: []provider.Upstream{{Name: &main, ExposedModels: []string{"gpt-4o", "gpt-4o"}}}},
		},
	}

	got := ResolveRoute(cfg, "gpt-4o")
	if !reflect.DeepEqual(got.Candidates, []string{"supplier-b"}) || got.Model != "gpt-4o" || got.Channel != "main" || got.Ambiguous {
		t.Fatalf("route = %+v, want unique supplier-b/main route", got)
	}

	cfg.Profiles["supplier-b"] = provider.Profile{Upstreams: []provider.Upstream{
		{Name: &main, ExposedModels: []string{"gpt-4o"}},
		{Name: &other, ExposedModels: []string{"gpt-4o"}},
	}}
	got = ResolveRoute(cfg, "gpt-4o")
	if len(got.Candidates) != 0 || got.Model != "gpt-4o" || !got.Ambiguous {
		t.Fatalf("duplicate channel route = %+v, want ambiguous", got)
	}

	cfg.Profiles["supplier-b"] = provider.Profile{Upstreams: []provider.Upstream{{Name: &main, ExposedModels: []string{"gpt-4o"}}}}
	cfg.Profiles["supplier-a"] = provider.Profile{Upstreams: []provider.Upstream{{Name: &main, ExposedModels: []string{"gpt-4o"}}}}
	got = ResolveRoute(cfg, "gpt-4o")
	if len(got.Candidates) != 0 || got.Model != "gpt-4o" || !got.Ambiguous {
		t.Fatalf("duplicate supplier route = %+v, want ambiguous", got)
	}
}

func TestResolveRouteLeavesMissingAndQualifiedIDsUnrouted(t *testing.T) {
	main := "main"
	cfg := configdomain.PiSwitchConfig{Profiles: map[string]provider.Profile{
		"supplier-a": {Upstreams: []provider.Upstream{{Name: &main, ExposedModels: []string{"gpt-4o"}}}},
	}}
	for _, requested := range []string{"missing", "supplier-a/gpt-4o", "supplier-a/main/gpt-4o"} {
		t.Run(requested, func(t *testing.T) {
			got := ResolveRoute(cfg, requested)
			if len(got.Candidates) != 0 || got.Model != requested || got.Channel != "" || got.Ambiguous {
				t.Fatalf("route = %+v, want no route for %q", got, requested)
			}
		})
	}
}
