package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestChannelAPI_OldConfigLoadWithoutAPI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	raw := `{"version":2,"profiles":{"oc":{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://opencode.ai/zen/go/v1","apiKey":"sk-test","models":[],"upstreams":[{"name":"chat","baseUrl":"https://opencode.ai/zen/go/v1","apiKey":"sk-test"}]}},"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}}`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadConfigAtPath(path)
	if err != nil {
		t.Fatalf("LoadConfigAtPath error: %v", err)
	}
	prof := cfg.Profiles["oc"]
	if len(prof.Upstreams) != 1 {
		t.Fatalf("upstreams = %d, want 1", len(prof.Upstreams))
	}
	if prof.Upstreams[0].API != "" {
		t.Fatalf("old config upstream API = %q, want empty (inherit)", prof.Upstreams[0].API)
	}
	// validate should not error for inherited api
	if err := ValidateUpstreamAPI(prof.Upstreams[0], prof); err != nil {
		t.Fatalf("validate inherited should be nil, got %v", err)
	}
	// MigratedForSave should keep empty api (inherit, not forced)
	migrated := MigratedForSave(cfg)
	if migrated.Profiles["oc"].Upstreams[0].API != "" {
		t.Fatalf("MigratedForSave should keep empty API for old config, got %q", migrated.Profiles["oc"].Upstreams[0].API)
	}
}

func TestChannelAPI_NewConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	chatName := "chat"
	respName := "responses"
	cfg.Profiles = map[string]ProviderProfile{
		"oc": {
			API:           "openai-completions",
			ResponsesMode: "auto",
			BaseURL:       "https://opencode.ai/zen/go/v1",
			APIKey:        "sk-test",
			Upstreams: []Upstream{
				{Name: &chatName, BaseURL: "https://opencode.ai/zen/go/v1", APIKey: "sk-test", API: "openai-completions", ResponsesMode: "convert"},
				{Name: &respName, BaseURL: "https://opencode.ai/zen/go/v1", APIKey: "sk-test2", API: "openai-responses", ResponsesMode: "passthrough"},
			},
		},
	}
	migrated := MigratedForSave(cfg)
	b, _ := json.Marshal(migrated)
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	profiles, _ := m["profiles"].(map[string]interface{})
	oc, _ := profiles["oc"].(map[string]interface{})
	ups, _ := oc["upstreams"].([]interface{})
	if len(ups) != 2 {
		t.Fatalf("marshaled upstreams = %d, want 2", len(ups))
	}
	u0, _ := ups[0].(map[string]interface{})
	if u0["api"] != "openai-completions" {
		t.Fatalf("u0 api = %v, want openai-completions", u0["api"])
	}
	u1, _ := ups[1].(map[string]interface{})
	if u1["api"] != "openai-responses" {
		t.Fatalf("u1 api = %v, want openai-responses", u1["api"])
	}
	// save and reload
	tmp := filepath.Join(dir, "rt.json")
	_ = os.WriteFile(tmp, b, 0644)
	loaded, _, err := LoadConfigAtPath(tmp)
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}
	p := loaded.Profiles["oc"]
	if p.Upstreams[0].API != "openai-completions" || p.Upstreams[1].API != "openai-responses" {
		t.Fatalf("reload APIs = %q, %q, want completions/responses", p.Upstreams[0].API, p.Upstreams[1].API)
	}
	if p.Upstreams[0].ResponsesMode != "convert" || p.Upstreams[1].ResponsesMode != "passthrough" {
		t.Fatalf("reload modes = %q, %q", p.Upstreams[0].ResponsesMode, p.Upstreams[1].ResponsesMode)
	}
}

func TestChannelAPI_ValidateInvalidAPI(t *testing.T) {
	prof := ProviderProfile{API: "openai-completions", ResponsesMode: "auto"}
	bad := Upstream{API: "invalid-api"}
	if err := ValidateUpstreamAPI(bad, prof); err == nil {
		t.Fatalf("expected error for invalid api, got nil")
	}
	// valid should pass
	ok := Upstream{API: "openai-responses", ResponsesMode: "auto"}
	if err := ValidateUpstreamAPI(ok, prof); err != nil {
		t.Fatalf("valid upstream should not error, got %v", err)
	}
}

func TestChannelAPI_ValidateIncompatibleMode(t *testing.T) {
	prof := ProviderProfile{API: "openai-completions", ResponsesMode: "auto"}
	// passthrough requires openai-responses
	bad := Upstream{API: "openai-completions", ResponsesMode: "passthrough"}
	if err := ValidateUpstreamAPI(bad, prof); err == nil {
		t.Fatalf("expected error for passthrough with completions, got nil")
	}
	// convert requires openai-completions
	bad2 := Upstream{API: "openai-responses", ResponsesMode: "convert"}
	if err := ValidateUpstreamAPI(bad2, prof); err == nil {
		t.Fatalf("expected error for convert with responses, got nil")
	}
	// inherited: upstream empty, profile is completions with passthrough should also fail when effective checked?
	// Upstream with no api inherits completions, but mode passthrough should fail
	bad3 := Upstream{ResponsesMode: "passthrough"}
	prof2 := ProviderProfile{API: "openai-completions", ResponsesMode: "auto"}
	if err := ValidateUpstreamAPI(bad3, prof2); err == nil {
		t.Fatalf("expected error for inherited completions + passthrough, got nil")
	}
}

func TestChannelAPI_EffectiveInheritance(t *testing.T) {
	prof := ProviderProfile{API: "openai-responses", ResponsesMode: "passthrough"}
	u := Upstream{API: "", ResponsesMode: ""}
	if got := u.EffectiveAPI(prof.API); got != "openai-responses" {
		t.Fatalf("EffectiveAPI = %q, want openai-responses", got)
	}
	if got := u.EffectiveResponsesMode(prof.ResponsesMode); got != "passthrough" {
		t.Fatalf("EffectiveResponsesMode = %q, want passthrough", got)
	}
	u2 := Upstream{API: "openai-completions", ResponsesMode: "convert"}
	if got := u2.EffectiveAPI(prof.API); got != "openai-completions" {
		t.Fatalf("override API = %q, want completions", got)
	}
}
