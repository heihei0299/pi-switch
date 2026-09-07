package gateway

import (
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func TestBareID_BuildProposedReturnsProvidersMapWithBareIDs(t *testing.T) {
	chat := "chat"
	resp := "responses"
	cfg := config.PiSwitchConfig{
		Settings: config.Settings{
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
			}{Host: "127.0.0.1", Port: 43112},
		},
		Profiles: map[string]config.ProviderProfile{
			"sup": {
				API: "openai-completions",
				Upstreams: []config.Upstream{
					{Name: &chat, API: "openai-completions", Models: []config.ModelEntry{{ID: "m1", ContextWindow: 100, MaxTokens: 10}}, ExposedModels: []string{"m1"}},
					{Name: &resp, API: "openai-responses", Models: []config.ModelEntry{{ID: "m1", ContextWindow: 200, MaxTokens: 20}}, ExposedModels: []string{"m1"}},
				},
			},
			"single": {
				API: "openai-completions",
				Upstreams: []config.Upstream{
					{Name: strptr("only"), API: "openai-completions", Models: []config.ModelEntry{{ID: "solo", ContextWindow: 128000, MaxTokens: 16384}}, ExposedModels: []string{"solo"}},
				},
			},
		},
	}
	proposed := BuildProposedGatewayEntry(cfg)
	// expect wrapper with providers
	provs, ok := proposed["providers"].(map[string]interface{})
	if !ok {
		t.Fatalf("proposed should have providers map, got %T %v", proposed["providers"], proposed)
	}
	// keys must be supplier/channel, no short
	if _, ok := provs["sup/chat"]; !ok {
		t.Fatalf("providers missing sup/chat: %v", keysOf(provs))
	}
	if _, ok := provs["sup/responses"]; !ok {
		t.Fatalf("providers missing sup/responses: %v", keysOf(provs))
	}
	if _, ok := provs["single/only"]; !ok {
		t.Fatalf("providers missing single/only (no short): %v", keysOf(provs))
	}
	if _, ok := provs["single"]; ok {
		t.Fatalf("should not have short single key, got %v", keysOf(provs))
	}
	// check bare ids
	for _, key := range []string{"sup/chat", "sup/responses", "single/only"} {
		entry, _ := provs[key].(map[string]interface{})
		models, _ := entry["models"].([]interface{})
		if len(models) != 1 {
			t.Fatalf("%s models len = %d want 1", key, len(models))
		}
		id, _ := models[0].(map[string]interface{})["id"].(string)
		if id == "" {
			t.Fatalf("%s model id empty", key)
		}
		if id != "m1" && id != "solo" {
			t.Fatalf("%s model id = %q want bare m1 or solo, not prefixed", key, id)
		}
		if containsSlash(id) {
			t.Fatalf("%s model id %q should be bare, no slash", key, id)
		}
	}
	// check per-channel api
	if e, _ := provs["sup/chat"].(map[string]interface{}); e["api"] != "openai-completions" {
		t.Fatalf("sup/chat api = %v want openai-completions", e["api"])
	}
	if e, _ := provs["sup/responses"].(map[string]interface{}); e["api"] != "openai-responses" {
		t.Fatalf("sup/responses api = %v want openai-responses", e["api"])
	}
	// ensure no old prefix ids anywhere
	for k, v := range provs {
		entry, _ := v.(map[string]interface{})
		models, _ := entry["models"].([]interface{})
		for _, m := range models {
			id := m.(map[string]interface{})["id"].(string)
			if id == k+"/m1" || id == k+"/solo" {
				t.Fatalf("found prefixed id %q", id)
			}
		}
	}
}

func keysOf(m map[string]interface{}) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
func containsSlash(s string) bool { return len(s) > 0 && (s[0] == '/' || indexSlash(s) >= 0) }
func indexSlash(s string) int {
	for i, c := range s {
		if c == '/' {
			return i
		}
	}
	return -1
}
