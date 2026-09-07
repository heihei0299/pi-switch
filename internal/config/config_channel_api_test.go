package config

import "testing"

func TestChannelAPI_IsRequired(t *testing.T) {
	prof := ProviderProfile{API: "openai-completions", ResponsesMode: "auto"}
	if err := ValidateUpstreamAPI(Upstream{}, prof); err == nil {
		t.Fatal("expected missing channel api error")
	}
	if err := ValidateUpstreamAPI(Upstream{API: "invalid-api"}, prof); err == nil {
		t.Fatal("expected invalid api error")
	}
	if err := ValidateUpstreamAPI(Upstream{API: "openai-responses"}, prof); err != nil {
		t.Fatalf("valid channel api error: %v", err)
	}
}

func TestChannelAPI_RoundTripKeepsIndependentAPIs(t *testing.T) {
	chat := "chat"
	responses := "responses"
	prof := ProviderProfile{Upstreams: []Upstream{
		{Name: &chat, API: "openai-completions"},
		{Name: &responses, API: "openai-responses"},
	}}
	if got := prof.Upstreams[0].API; got != "openai-completions" {
		t.Fatalf("chat api = %q", got)
	}
	if got := prof.Upstreams[1].API; got != "openai-responses" {
		t.Fatalf("responses api = %q", got)
	}
}
