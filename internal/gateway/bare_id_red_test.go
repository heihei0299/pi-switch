package gateway

import (
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func TestBareID_BuildProposedReturnsFixedProvidersWithBareIDs(t *testing.T) {
	chat := "chat"
	resp := "responses"
	cfg := config.PiSwitchConfig{
		Settings: config.Settings{
			Proxy: struct {
				Host                          string                        `json:"host"`
				Port                          int                           `json:"port"`
				UserAgent                     *string                       `json:"userAgent,omitempty"`
				CircuitBreaker                config.CircuitBreakerSettings `json:"circuitBreaker"`
				RequestRetry                  *int                          `json:"requestRetry,omitempty"`
				MaxRetryCredentials           int                           `json:"maxRetryCredentials,omitempty"`
				MaxRetryInterval              *int                          `json:"maxRetryInterval,omitempty"`
				DisableCooling                *bool                         `json:"disableCooling,omitempty"`
				TransientErrorCooldownSeconds *int                          `json:"transientErrorCooldownSeconds,omitempty"`
				RequestScopedErrors           []config.RequestScopedError   `json:"requestScopedErrors,omitempty"`
			}{Host: "127.0.0.1", Port: 43112},
		},
		Profiles: map[string]config.ProviderProfile{
			"sup": {
				API: "openai-completions",
				Upstreams: []config.Upstream{
					{Name: &chat, API: "openai-completions", Models: []config.ModelEntry{{ID: "m1", ContextWindow: 100, MaxTokens: 10}}, ExposedModels: []string{"m1"}},
					{Name: &resp, API: "openai-responses", Models: []config.ModelEntry{{ID: "r1", ContextWindow: 200, MaxTokens: 20}}, ExposedModels: []string{"r1"}},
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
	provs, ok := proposed["providers"].(map[string]interface{})
	if !ok {
		t.Fatalf("proposed should have providers map, got %T %v", proposed["providers"], proposed)
	}
	if len(provs) != 2 {
		t.Fatalf("providers len = %d want 2: %v", len(provs), keysOf(provs))
	}
	for _, key := range []string{"pi-switch-chat", "pi-switch-res"} {
		if _, ok := provs[key]; !ok {
			t.Fatalf("providers missing %s: %v", key, keysOf(provs))
		}
	}
	for _, key := range []string{"sup/chat", "sup/responses", "single/only"} {
		if _, ok := provs[key]; ok {
			t.Fatalf("legacy provider %s should not be present: %v", key, keysOf(provs))
		}
	}
	checkModels := func(key string, want []string) {
		entry := provs[key].(map[string]interface{})
		models := entry["models"].([]interface{})
		if len(models) != len(want) {
			t.Fatalf("%s models len = %d want %d", key, len(models), len(want))
		}
		got := make([]string, 0, len(models))
		for _, raw := range models {
			id := raw.(map[string]interface{})["id"].(string)
			if containsSlash(id) {
				t.Fatalf("%s model id %q should be bare, no slash", key, id)
			}
			got = append(got, id)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s model ids = %v want %v", key, got, want)
			}
		}
	}
	checkModels("pi-switch-chat", []string{"m1", "solo"})
	checkModels("pi-switch-res", []string{"r1"})
	if e := provs["pi-switch-chat"].(map[string]interface{}); e["api"] != "openai-completions" {
		t.Fatalf("pi-switch-chat api = %v want openai-completions", e["api"])
	}
	if e := provs["pi-switch-res"].(map[string]interface{}); e["api"] != "openai-responses" {
		t.Fatalf("pi-switch-res api = %v want openai-responses", e["api"])
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
