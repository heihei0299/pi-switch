package inference

import (
	"errors"
	"testing"

	configdomain "github.com/heihei0299/pi-switch/internal/domain/config"
	"github.com/heihei0299/pi-switch/internal/domain/protocol"
	"github.com/heihei0299/pi-switch/internal/domain/provider"
)

func TestPrepareRequestRoutesGloballyAndBuildsProtocolBody(t *testing.T) {
	current, main := "supplier-a", "main"
	cfg := configdomain.PiSwitchConfig{
		Current: &current,
		Profiles: map[string]provider.Profile{
			"supplier-a": {Upstreams: []provider.Upstream{{Name: &main, API: protocol.OpenAIChat, ExposedModels: []string{"other"}}}},
			"supplier-b": {Upstreams: []provider.Upstream{{
				Name: &main, API: protocol.OpenAIChat,
				BaseURL: "https://upstream.example/v1", APIKey: "channel-key",
				Models:        []provider.ModelEntry{{ID: "gpt-4o", ContextWindow: 128000, MaxTokens: 16384}},
				ExposedModels: []string{"gpt-4o"},
			}}},
		},
	}

	prepared, err := PrepareRequest(cfg, "gpt-4o", "responses", map[string]interface{}{
		"input": "hi", "max_output_tokens": float64(999999),
	}, 400, false)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Supplier != "supplier-b" || prepared.Model != "gpt-4o" || prepared.Profile.API != protocol.OpenAIChat {
		t.Fatalf("prepared route = %+v", prepared)
	}
	if prepared.Upstream.BaseURL != "https://upstream.example/v1" || prepared.Upstream.APIKey != "channel-key" || prepared.ModelEntry == nil {
		t.Fatalf("prepared upstream/model = %+v / %+v", prepared.Upstream, prepared.ModelEntry)
	}
	if prepared.ProtocolPlan.UpstreamPath != "/v1/chat/completions" || !prepared.ProtocolPlan.NeedsConvert() {
		t.Fatalf("protocol plan = %+v", prepared.ProtocolPlan)
	}
	if prepared.Body["model"] != "gpt-4o" || prepared.Body["max_tokens"] != float64(16384) {
		t.Fatalf("upstream body = %#v", prepared.Body)
	}
}

func TestPrepareRequestSelectsTheChannelExposingTheModel(t *testing.T) {
	main, backup := "main", "backup"
	cfg := configdomain.PiSwitchConfig{Profiles: map[string]provider.Profile{
		"supplier": {Upstreams: []provider.Upstream{
			{Name: &main, API: protocol.OpenAIChat, BaseURL: "https://main.example/v1", APIKey: "main-key", Models: []provider.ModelEntry{{ID: "other", ContextWindow: 128000, MaxTokens: 100}}, ExposedModels: []string{"other"}},
			{Name: &backup, API: protocol.OpenAIChat, BaseURL: "https://backup.example/v1", APIKey: "backup-key", Models: []provider.ModelEntry{{ID: "gpt-4o", ContextWindow: 128000, MaxTokens: 42}}, ExposedModels: []string{"gpt-4o"}},
		}},
	}}

	prepared, err := PrepareRequest(cfg, "gpt-4o", "chat", map[string]interface{}{"messages": []interface{}{}, "max_tokens": float64(999)}, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Upstream.BaseURL != "https://backup.example/v1" || prepared.Upstream.APIKey != "backup-key" || prepared.ModelEntry == nil || prepared.Body["max_tokens"] != float64(42) {
		t.Fatalf("prepared channel = upstream %+v, model %+v, body %#v; want backup route and its token limit", prepared.Upstream, prepared.ModelEntry, prepared.Body)
	}
}

func TestPrepareRequestStreamingMarksBodyAndRejectsMissingStreamConverter(t *testing.T) {
	main := "main"
	cfg := configdomain.PiSwitchConfig{Profiles: map[string]provider.Profile{
		"supplier": {Upstreams: []provider.Upstream{{
			Name: &main, API: protocol.OpenAIChat,
			BaseURL:       "https://upstream.example/v1",
			Models:        []provider.ModelEntry{{ID: "gpt-4o", ContextWindow: 128000, MaxTokens: 16384}},
			ExposedModels: []string{"gpt-4o"},
		}}},
	}}
	prepared, err := PrepareRequest(cfg, "gpt-4o", "chat", map[string]interface{}{"messages": []interface{}{}}, 100, true)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Body["stream"] != true {
		t.Fatalf("stream flag = %#v", prepared.Body["stream"])
	}

	cfg.Profiles["supplier"] = provider.Profile{Upstreams: []provider.Upstream{{
		Name: &main, API: protocol.OpenAIChat,
		BaseURL:       "https://upstream.example/v1",
		Models:        []provider.ModelEntry{{ID: "gpt-4o", ContextWindow: 128000, MaxTokens: 16384}},
		ExposedModels: []string{"gpt-4o"},
	}}}
	_, err = PrepareRequest(cfg, "gpt-4o", "messages", map[string]interface{}{"messages": []interface{}{}}, 100, true)
	var requestError *PreparationError
	if !errors.As(err, &requestError) || requestError.Type != "not_supported" {
		t.Fatalf("unsupported stream conversion error = %v", err)
	}
}

func TestPreparedRequestBuildsTheExistingMinTokenRetryBody(t *testing.T) {
	main := "main"
	cfg := configdomain.PiSwitchConfig{Profiles: map[string]provider.Profile{
		"supplier": {Upstreams: []provider.Upstream{{
			Name: &main, API: protocol.OpenAIResponses,
			BaseURL:       "https://upstream.example/v1",
			Models:        []provider.ModelEntry{{ID: "gpt-4o", ContextWindow: 128000, MaxTokens: 16384}},
			ExposedModels: []string{"gpt-4o"},
		}}},
	}}
	prepared, err := PrepareRequest(cfg, "gpt-4o", "responses", map[string]interface{}{"input": "hi"}, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	body, err := prepared.MinTokenRetryBody()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"max_tokens", "max_output_tokens", "max_completion_tokens"} {
		if body[key] != float64(16) {
			t.Errorf("retry %s = %#v, want 16", key, body[key])
		}
	}
}

func TestPrepareRequestClampsEncryptedInputAndLeavesAbsentOrUnknownLimitsAlone(t *testing.T) {
	main := "main"
	model := provider.ModelEntry{ID: "gpt-4o", ContextWindow: 1048576, MaxTokens: 943718}
	cfg := configdomain.PiSwitchConfig{Profiles: map[string]provider.Profile{
		"supplier": {Upstreams: []provider.Upstream{{
			Name: &main, API: protocol.OpenAIChat, BaseURL: "https://upstream.example/v1",
			Models: []provider.ModelEntry{model}, ExposedModels: []string{"gpt-4o"},
		}}},
	}}
	encrypted := string(make([]byte, 500000))
	prepared, err := PrepareRequest(cfg, "gpt-4o", "chat", map[string]interface{}{
		"messages":          []interface{}{map[string]interface{}{"encrypted_content": encrypted}},
		"max_output_tokens": float64(908720),
	}, 855000, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := prepared.Body["max_output_tokens"].(float64); got >= 800000 || got >= 908720 {
		t.Fatalf("encrypted-content output limit = %v, want below 800000", got)
	}

	prepared, err = PrepareRequest(cfg, "gpt-4o", "chat", map[string]interface{}{"messages": []interface{}{}}, 200, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := prepared.Body["max_output_tokens"]; ok {
		t.Fatalf("missing max_output_tokens was injected: %#v", prepared.Body)
	}

	for _, entry := range []*provider.ModelEntry{
		nil,
		{ID: "gpt-4o", MaxTokens: 16},
		{ID: "gpt-4o", ContextWindow: 128000},
	} {
		upstream := provider.Upstream{Name: &main, API: protocol.OpenAIChat, BaseURL: "https://upstream.example/v1", ExposedModels: []string{"gpt-4o"}}
		if entry != nil {
			upstream.Models = []provider.ModelEntry{*entry}
		}
		cfg.Profiles["supplier"] = provider.Profile{API: protocol.OpenAIChat, Upstreams: []provider.Upstream{upstream}}
		body := map[string]interface{}{"max_tokens": float64(999999), "max_output_tokens": float64(888888), "max_completion_tokens": float64(777777)}
		prepared, err := PrepareRequest(cfg, "gpt-4o", "chat", body, 100, false)
		if err != nil {
			t.Fatal(err)
		}
		for key, want := range map[string]float64{"max_tokens": 999999, "max_output_tokens": 888888, "max_completion_tokens": 777777} {
			if got := prepared.Body[key]; got != want {
				t.Errorf("entry=%+v %s = %v, want %v", entry, key, got, want)
			}
		}
	}
}
