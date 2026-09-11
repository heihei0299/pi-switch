package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func TestMergeGatewayExtraMigratesLegacyModelFieldsWithFixedPriority(t *testing.T) {
	current := map[string]interface{}{"providers": map[string]interface{}{
		"legacy/main": map[string]interface{}{
			"api": "openai-completions", "apiKey": "pi-switch-proxy", "headers": map[string]interface{}{"old": "provider"},
			"models": []interface{}{map[string]interface{}{
				"id": "m1", "compat": map[string]interface{}{"legacy": true, "shared": "legacy"}, "legacyField": "keep",
			}},
		},
		gatewayChatProvider: map[string]interface{}{
			"api": "openai-completions", "apiKey": "pi-switch-proxy",
			"models": []interface{}{map[string]interface{}{
				"id": "m1", "compat": map[string]interface{}{"fixed": true, "shared": "fixed"}, "fixedField": "wins",
			}},
		},
	}}
	proposed := map[string]interface{}{"providers": map[string]interface{}{
		gatewayChatProvider: map[string]interface{}{
			"api": "openai-completions", "apiKey": "pi-switch-proxy",
			"models": []interface{}{map[string]interface{}{"id": "m1", "contextWindow": float64(100)}},
		},
	}}
	merged := MergeGatewayExtra(current, proposed)
	providers := merged["providers"].(map[string]interface{})
	entry := providers[gatewayChatProvider].(map[string]interface{})
	model := entry["models"].([]interface{})[0].(map[string]interface{})
	compat := model["compat"].(map[string]interface{})
	if compat["fixed"] != true || compat["shared"] != "fixed" || compat["legacy"] != true {
		t.Fatalf("compat migration/priority = %#v", compat)
	}
	if model["legacyField"] != "keep" || model["fixedField"] != "wins" {
		t.Fatalf("model fields = %#v", model)
	}
	if _, ok := entry["headers"]; ok {
		t.Fatalf("legacy provider-level headers migrated: %#v", entry["headers"])
	}
}

func TestMergeGatewayExtraForConfigRecognizesManagedLegacyProvider(t *testing.T) {
	legacy := "main"
	cfg := config.PiSwitchConfig{Profiles: map[string]config.ProviderProfile{
		"sup": {Upstreams: []config.Upstream{{Name: &legacy, API: "openai-completions", ExposedModels: []string{"m1"}}}},
	}}
	current := map[string]interface{}{"providers": map[string]interface{}{
		"sup/main": map[string]interface{}{"models": []interface{}{map[string]interface{}{"id": "m1", "manual": true}}},
	}}
	proposed := map[string]interface{}{"providers": map[string]interface{}{
		gatewayChatProvider: map[string]interface{}{"models": []interface{}{map[string]interface{}{"id": "m1"}}},
	}}
	merged := MergeGatewayExtraForConfig(cfg, current, proposed)
	model := merged["providers"].(map[string]interface{})[gatewayChatProvider].(map[string]interface{})["models"].([]interface{})[0].(map[string]interface{})
	if model["manual"] != true {
		t.Fatalf("managed legacy model fields = %#v", model)
	}
}

func TestPublishRemovesLegacyProvidersAndKeepsThirdParty(t *testing.T) {
	modelsPath := filepath.Join(t.TempDir(), "models.json")
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	current := map[string]interface{}{"providers": map[string]interface{}{
		"legacy/main": map[string]interface{}{
			"api": "openai-completions", "apiKey": "pi-switch-proxy",
			"models": []interface{}{map[string]interface{}{"id": "m1", "legacyField": "migrated"}},
		},
		"third-party": map[string]interface{}{
			"api": "openai-completions", "apiKey": "third-key", "models": []interface{}{map[string]interface{}{"id": "other"}},
		},
	}}
	initial, _ := json.Marshal(current)
	if err := os.WriteFile(modelsPath, initial, 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.PiSwitchConfig{Profiles: map[string]config.ProviderProfile{
		"legacy": {Upstreams: []config.Upstream{{Name: strptr("main"), API: "openai-completions", ExposedModels: []string{"m1"}}}},
	}}
	edited := map[string]interface{}{"providers": map[string]interface{}{
		gatewayChatProvider: map[string]interface{}{
			"api": "openai-completions", "baseUrl": "http://127.0.0.1:43112/v1", "apiKey": "pi-switch-proxy",
			"models": []interface{}{map[string]interface{}{"id": "m1"}},
		},
	}}
	if err := publishDraft(t, cfg, edited); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	providers := got["providers"].(map[string]interface{})
	if _, ok := providers["legacy/main"]; ok {
		t.Fatalf("legacy provider was not removed: %#v", providers)
	}
	if _, ok := providers["third-party"]; !ok {
		t.Fatalf("third-party provider was removed: %#v", providers)
	}
	model := providers[gatewayChatProvider].(map[string]interface{})["models"].([]interface{})[0].(map[string]interface{})
	if model["legacyField"] != "migrated" {
		t.Fatalf("legacy model field not migrated: %#v", model)
	}
}
