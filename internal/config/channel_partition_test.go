package config

import "testing"

func upName(s string) *string { return &s }

func testPartitionProfile() ProviderProfile {
	return ProviderProfile{
		API:     "openai-completions",
		BaseURL: "https://api.example.com/v1",
		APIKey:  "sk-test",
		Upstreams: []Upstream{
			{
				Name:          upName("main"),
				API:           "openai-completions",
				BaseURL:       "https://api.example.com/v1",
				APIKey:        "sk-test",
				Models:        []ModelEntry{{ID: "m1"}, {ID: "m2"}},
				ExposedModels: []string{"m1"},
			},
			{
				Name:    upName("backup"),
				API:     "openai-completions",
				BaseURL: "https://backup.example.com/v1",
				APIKey:  "sk-test2",
			},
		},
	}
}

func TestChannelPartition_ChannelViewReadsOnlyNamedChannel(t *testing.T) {
	prof := testPartitionProfile()
	models, exposed := prof.ChannelView("main")
	if len(models) != 2 || len(exposed) != 1 || exposed[0] != "m1" {
		t.Fatalf("main view = (%v, %v)", models, exposed)
	}
	models, exposed = prof.ChannelView("backup")
	if len(models) != 0 || len(exposed) != 0 {
		t.Fatalf("backup view = (%v, %v), want empty", models, exposed)
	}
}

func TestChannelPartition_MigratedForSaveDoesNotCreateLegacyFields(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Profiles = map[string]ProviderProfile{"p": testPartitionProfile()}
	got := MigratedForSave(cfg).Profiles["p"]
	if len(got.Upstreams) != 2 || len(got.Upstreams[0].Models) != 2 {
		t.Fatalf("upstreams changed during save: %+v", got.Upstreams)
	}
}

func TestChannelPartition_ChannelNamesAreRequiredForGatewayIdentity(t *testing.T) {
	prof := testPartitionProfile()
	prof.Upstreams[0].Name = nil
	if got := prof.ChannelName(0); got != "" {
		t.Fatalf("unnamed channel = %q, want empty", got)
	}
}
