package provider

import (
	"reflect"
	"testing"
)

func TestNarrowToChannelPreservesEffectiveProviderSettings(t *testing.T) {
	main, alternate := "main", "alternate"
	profile := Profile{
		API:           "openai-completions",
		ResponsesMode: "convert",
		BaseURL:       "https://profile.example/v1",
		APIKey:        "profile-key",
		Headers:       map[string]string{"X-Profile": "profile"},
		Upstreams: []Upstream{
			{Name: &main},
			{Name: &alternate, API: "openai-responses", ResponsesMode: "passthrough", BaseURL: "https://alternate.example/v1", APIKey: "alternate-key"},
		},
	}

	primary := profile.NarrowToChannel(0)
	if primary.API != profile.API || primary.ResponsesMode != profile.ResponsesMode || len(primary.Upstreams) != 1 || primary.Upstreams[0].Name == nil || *primary.Upstreams[0].Name != main {
		t.Fatalf("primary profile = %+v", primary)
	}
	if got := primary.PrimaryUpstream(); got.BaseURL != profile.BaseURL || got.APIKey != profile.APIKey || !reflect.DeepEqual(got.Headers, profile.Headers) {
		t.Fatalf("primary upstream = %+v, want profile fallbacks", got)
	}

	secondary := profile.NarrowToChannel(1)
	if secondary.API != "openai-responses" || secondary.ResponsesMode != "passthrough" || len(secondary.Upstreams) != 1 {
		t.Fatalf("secondary profile = %+v", secondary)
	}
	if got := secondary.PrimaryUpstream(); got.BaseURL != "https://alternate.example/v1" || got.APIKey != "alternate-key" {
		t.Fatalf("secondary upstream = %+v", got)
	}

	if got := profile.NarrowToChannel(2); !reflect.DeepEqual(got, profile) {
		t.Fatalf("out-of-range channel changed profile: %+v", got)
	}
}

func TestPrimaryUpstreamResolvesLegacyFlatProfile(t *testing.T) {
	profile := Profile{BaseURL: "https://legacy.example/v1", APIKey: "legacy-key", Headers: map[string]string{"X-Legacy": "yes"}}
	got := profile.PrimaryUpstream()
	if got.BaseURL != profile.BaseURL || got.APIKey != profile.APIKey || !reflect.DeepEqual(got.Headers, profile.Headers) {
		t.Fatalf("primary upstream = %+v, want legacy flat settings", got)
	}
}
