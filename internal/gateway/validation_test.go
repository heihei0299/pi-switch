package gateway

import (
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func TestValidateProposedGatewayRejectsDuplicateExposedModel(t *testing.T) {
	a, b := "main", "backup"
	cfg := config.PiSwitchConfig{Profiles: map[string]config.ProviderProfile{
		"alpha": {Upstreams: []config.Upstream{{Name: &a, API: "openai-completions", ExposedModels: []string{"same"}}}},
		"beta":  {Upstreams: []config.Upstream{{Name: &b, API: "openai-completions", ExposedModels: []string{"same"}}}},
	}}
	proposed := map[string]interface{}{"providers": map[string]interface{}{
		gatewayChatProvider: map[string]interface{}{
			"api": "openai-completions", "baseUrl": "http://127.0.0.1:43112/v1", "apiKey": "pi-switch-proxy",
			"models": []interface{}{map[string]interface{}{"id": "same"}},
		},
	}}
	if err := ValidateProposedGateway(cfg, proposed); err == nil || !strings.Contains(err.Error(), "same") || !strings.Contains(err.Error(), "alpha/main") || !strings.Contains(err.Error(), "beta/backup") {
		t.Fatalf("duplicate validation error = %v", err)
	}
}

func TestValidateProposedGatewayRejectsThirdProxyProvider(t *testing.T) {
	cfg := config.PiSwitchConfig{}
	proposed := map[string]interface{}{"providers": map[string]interface{}{
		"pi-switch-extra": map[string]interface{}{
			"api": "openai-completions", "baseUrl": "http://127.0.0.1:43112/v1", "apiKey": "pi-switch-proxy",
			"models": []interface{}{},
		},
	}}
	if err := ValidateProposedGateway(cfg, proposed); err == nil || !strings.Contains(err.Error(), "pi-switch-extra") {
		t.Fatalf("third provider validation error = %v", err)
	}
}

func TestBuildGatewayDiagnosticsReportsUnsupportedChannels(t *testing.T) {
	unsupported := "anthropic"
	cfg := config.PiSwitchConfig{Profiles: map[string]config.ProviderProfile{
		"sup": {Upstreams: []config.Upstream{{Name: &unsupported, API: "anthropic-messages", ExposedModels: []string{"claude"}}}},
	}}
	diagnostics := BuildGatewayDiagnostics(cfg)
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one unsupported API diagnostic", diagnostics)
	}
	if diagnostics[0].Supplier != "sup" || diagnostics[0].Channel != unsupported || diagnostics[0].API != "anthropic-messages" {
		t.Fatalf("diagnostic = %#v", diagnostics[0])
	}
	if diagnostics[0].Code != "unsupported-api" {
		t.Fatalf("diagnostic code = %q", diagnostics[0].Code)
	}
}
