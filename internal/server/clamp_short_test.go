package server

import (
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func TestClampBody_NotInjectedWhenAbsent(t *testing.T) {
	prof := config.ProviderProfile{
		Upstreams: []config.Upstream{{Models: []config.ModelEntry{{ID: "muse-spark-1.2-contributor", ContextWindow: 1048576, MaxTokens: 943718}}}},
	}
	entry := prof.Upstreams[0].Models[0]
	body := map[string]interface{}{
		"model": "muse-spark-1.2-contributor",
		"input": "hi",
	}
	origLen := 200
	clampBody(body, &entry, origLen)
	if _, ok := body["max_output_tokens"]; ok {
		t.Fatalf("should not inject max_output_tokens when absent, got %v", body["max_output_tokens"])
	}
	if _, ok := body["max_tokens"]; ok {
		t.Fatalf("should not inject max_tokens when absent")
	}
}

func TestClampBody_EncryptedContentConservative(t *testing.T) {
	prof := config.ProviderProfile{
		Upstreams: []config.Upstream{{Models: []config.ModelEntry{{ID: "muse-spark-1.2-contributor", ContextWindow: 1048576, MaxTokens: 943718}}}},
	}
	entry := prof.Upstreams[0].Models[0]
	// Simulate body with encrypted_content of 500k length, total rawLen 855k, requested 908720
	// effectiveLen = 855k + 0.2*500k = 955k, est=318k, available=722k, clamped ~722k
	encrypted := string(make([]byte, 500000))
	body := map[string]interface{}{
		"model": "muse-spark-1.2-contributor",
		"input": []interface{}{
			map[string]interface{}{"type": "reasoning", "encrypted_content": encrypted},
		},
		"max_output_tokens": float64(908720),
	}
	clampedBefore := 908720
	clampBody(body, &entry, 855000)
	v, ok := body["max_output_tokens"]
	if !ok {
		t.Fatalf("max_output_tokens should still exist")
	}
	got := int(v.(float64))
	if got >= clampedBefore {
		t.Fatalf("should clamp down from 908720, got %d", got)
	}
	if got > 800000 {
		t.Fatalf("clamped too high for encrypted_content case, got %d want <800k", got)
	}
}
