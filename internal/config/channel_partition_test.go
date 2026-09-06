package config

import (
	"testing"
)

// S1：渠道分区存储与迁移。Upstream 增量 models/exposedModels 分区字段；
// 读取分区优先、回退顶层；保存时顶层自动迁入首条渠道。

func upName(s string) *string { return &s }

func testPartitionProfile() ProviderProfile {
	return ProviderProfile{
		API:     "openai-completions",
		BaseURL: "https://api.example.com/v1",
		APIKey:  "sk-test",
		Models: []ModelEntry{
			{ID: "m1", ContextWindow: 128000, MaxTokens: 16384},
			{ID: "m2", ContextWindow: 128000, MaxTokens: 16384},
		},
		ExposedModels: []string{"m1"},
		Upstreams: []Upstream{
			{Name: upName("main"), BaseURL: "https://api.example.com/v1", APIKey: "sk-test"},
			{Name: upName("backup"), BaseURL: "https://backup.example.com/v1", APIKey: "sk-test2"},
		},
	}
}

func TestChannelPartition_SaveMigratesTopLevelToFirstChannel(t *testing.T) {
	cfg := DefaultConfig()
	prof := testPartitionProfile()
	cfg.Profiles = map[string]ProviderProfile{"p": prof}

	migrated := MigratedForSave(cfg)
	got := migrated.Profiles["p"]
	if len(got.Models) != 0 {
		t.Fatalf("top-level models after migrate = %d, want 0 (moved)", len(got.Models))
	}
	if len(got.ExposedModels) != 0 {
		t.Fatalf("top-level exposed after migrate = %d, want 0 (moved)", len(got.ExposedModels))
	}
	if len(got.Upstreams) != 2 {
		t.Fatalf("upstreams = %d, want 2", len(got.Upstreams))
	}
	first := got.Upstreams[0]
	if len(first.Models) != 2 {
		t.Fatalf("first channel models = %d, want 2", len(first.Models))
	}
	if len(first.ExposedModels) != 1 || first.ExposedModels[0] != "m1" {
		t.Fatalf("first channel exposed = %v, want [m1]", first.ExposedModels)
	}
	if len(got.Upstreams[1].Models) != 0 {
		t.Fatalf("second channel models = %d, want 0 (empty pool)", len(got.Upstreams[1].Models))
	}
}

func TestChannelPartition_LoadFallsBackToTopLevel(t *testing.T) {
	prof := testPartitionProfile()
	models, exposed := prof.ChannelView("main")
	if len(models) != 2 {
		t.Fatalf("fallback models = %d, want 2", len(models))
	}
	if len(exposed) != 1 || exposed[0] != "m1" {
		t.Fatalf("fallback exposed = %v, want [m1]", exposed)
	}
}

func TestChannelPartition_PartitionPreferredOverTopLevel(t *testing.T) {
	prof := testPartitionProfile()
	prof.Upstreams[0].Models = []ModelEntry{{ID: "c1", ContextWindow: 100, MaxTokens: 10}}
	prof.Upstreams[0].ExposedModels = []string{"c1"}
	models, exposed := prof.ChannelView("main")
	if len(models) != 1 || models[0].ID != "c1" {
		t.Fatalf("partition models = %v, want [c1]", models)
	}
	if len(exposed) != 1 || exposed[0] != "c1" {
		t.Fatalf("partition exposed = %v, want [c1]", exposed)
	}
	// 未分区的另一渠道不回退顶层（跨渠道隔离），返回空
	models2, exposed2 := prof.ChannelView("backup")
	if len(models2) != 0 || len(exposed2) != 0 {
		t.Fatalf("unpartitioned channel = (%v,%v), want empty", models2, exposed2)
	}
}

func TestChannelPartition_NoUpstreamsKeepsTopLevel(t *testing.T) {
	cfg := DefaultConfig()
	prof := testPartitionProfile()
	prof.Upstreams = nil
	cfg.Profiles = map[string]ProviderProfile{"p": prof}
	migrated := MigratedForSave(cfg)
	got := migrated.Profiles["p"]
	if len(got.Models) != 2 {
		t.Fatalf("no-upstream migrate models = %d, want 2 (kept)", len(got.Models))
	}
	models, _ := got.ChannelView("")
	if len(models) != 2 {
		t.Fatalf("no-upstream view models = %d, want 2", len(models))
	}
}

func TestChannelPartition_AlreadyPartitionedNoDoubleMigrate(t *testing.T) {
	cfg := DefaultConfig()
	prof := testPartitionProfile()
	prof.Upstreams[0].Models = []ModelEntry{{ID: "c1", ContextWindow: 100, MaxTokens: 10}}
	prof.Upstreams[0].ExposedModels = []string{"c1"}
	cfg.Profiles = map[string]ProviderProfile{"p": prof}
	migrated := MigratedForSave(cfg)
	got := migrated.Profiles["p"]
	// 已分区：顶层残留不动（本轮仅迁移全空分区情形），分区不被覆盖
	if len(got.Upstreams[0].Models) != 1 || got.Upstreams[0].Models[0].ID != "c1" {
		t.Fatalf("partition overwritten: %v", got.Upstreams[0].Models)
	}
	if len(got.Models) != 2 {
		t.Fatalf("top-level touched = %d, want 2 (kept when partitioned)", len(got.Models))
	}
}
