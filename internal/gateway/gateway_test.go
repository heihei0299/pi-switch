package gateway

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func strptr(s string) *string { return &s }
func boolptr(b bool) *bool    { return &b }

// Regression: 落盘 cost 含显式 cacheWrite:0（经 webui draft round-trip 写入）
// 而 BuildProposed 用 *ModelCost（omitempty 省掉 0），导致 models 恒 changed、
// preview pending 恒=1、“立即同步”点了再进还弹。
// 提议侧必须显式带 cacheWrite，写后读才能收敛到 pending=0。
func TestProposedCostIncludesZeroCacheWrite(t *testing.T) {
	cfg := config.PiSwitchConfig{
		Settings: config.Settings{
			GatewayAPI: "openai-responses",
		},
		Profiles: map[string]config.ProviderProfile{
			"oc": {
				Models: []config.ModelEntry{
					{
						ID: "mimo-v2.5", ContextWindow: 1048576, MaxTokens: 131072,
						Name:      strptr("MiMo-V2.5"),
						Cost:      &config.ModelCost{Input: 0.4, Output: 2, CacheRead: 0.08},
						Input:     []string{"text", "image"},
						Reasoning: boolptr(true),
					},
				},
				ExposedModels: []string{"mimo-v2.5"},
			},
		},
	}
	cfg.Settings.Proxy.Host = "127.0.0.1"
	cfg.Settings.Proxy.Port = 43112

	proposed := BuildProposedGatewayEntry(cfg)
	models, _ := proposed["models"].([]interface{})
	if len(models) != 1 {
		t.Fatalf("proposed models len = %d, want 1", len(models))
	}
	costJSON, _ := json.Marshal(models[0].(map[string]interface{})["cost"])
	if !strings.Contains(string(costJSON), `"cacheWrite"`) {
		t.Fatalf("proposed cost omits cacheWrite: %s (want explicit cacheWrite:0)", string(costJSON))
	}
}

// 端到端收敛：current 为“已发布一次后的落盘形态”（cost 含 cacheWrite:0），
// 经 MergeGatewayExtra 归一化后 ComputePendingCount 必须为 0。
func TestPreviewPendingConvergesAfterPublish(t *testing.T) {
	cfg := config.PiSwitchConfig{
		Settings: config.Settings{
			GatewayAPI: "openai-responses",
		},
		Profiles: map[string]config.ProviderProfile{
			"oc": {
				Models: []config.ModelEntry{
					{
						ID: "mimo-v2.5", ContextWindow: 1048576, MaxTokens: 131072,
						Name:      strptr("MiMo-V2.5"),
						Cost:      &config.ModelCost{Input: 0.4, Output: 2, CacheRead: 0.08},
						Input:     []string{"text", "image"},
						Reasoning: boolptr(true),
					},
				},
				ExposedModels: []string{"mimo-v2.5"},
			},
		},
	}
	cfg.Settings.Proxy.Host = "127.0.0.1"
	cfg.Settings.Proxy.Port = 43112

	proposed := BuildProposedGatewayEntry(cfg)
	// 模拟落盘 current：与 proposed 同构，但 cost 系 webui draft 形态（含 cacheWrite:0）
	current := map[string]interface{}{
		"api":     "openai-responses",
		"baseUrl": "http://127.0.0.1:43112/v1",
		"apiKey":  "pi-switch-proxy",
		"proxy":   false,
		"models": []interface{}{
			map[string]interface{}{
				"id":            "oc/mimo-v2.5",
				"contextWindow": float64(1048576),
				"maxTokens":     float64(131072),
				"name":          "MiMo-V2.5",
				"cost": map[string]interface{}{
					"input": 0.4, "output": float64(2),
					"cacheRead": 0.08, "cacheWrite": float64(0),
				},
				"input":  []interface{}{"text", "image"},
				"compat": map[string]interface{}{"supportsDeveloperRole": false},
			},
		},
	}
	merged := MergeGatewayExtra(current, proposed)
	if got := ComputePendingCount(current, merged); got != 0 {
		a, r, c := DiffGateway(current, merged)
		t.Fatalf("pending = %d, want 0 (added=%v removed=%v changed=%v)", got, a, r, c)
	}
}

// S5：分区聚合前缀 id。已分区 profile 按渠道逐条聚合，id = supplier/channel/modelId；
// 跨渠道同模型 id 为独立条目；未分区保持 supplier/modelId 旧形态。
func TestProposedChannelPrefixedIDs(t *testing.T) {
	cfg := config.PiSwitchConfig{
		Settings: config.Settings{GatewayAPI: "openai-completions"},
		Profiles: map[string]config.ProviderProfile{
			"sup": {
				Upstreams: []config.Upstream{
					{
						Name:          strptr("main"),
						Models:        []config.ModelEntry{{ID: "m1", ContextWindow: 100, MaxTokens: 10}},
						ExposedModels: []string{"m1"},
					},
					{
						Name:          strptr("bk"),
						Models:        []config.ModelEntry{{ID: "m1", ContextWindow: 200, MaxTokens: 20}},
						ExposedModels: []string{"m1"},
					},
				},
			},
			"legacy": {
				Models:        []config.ModelEntry{{ID: "old", ContextWindow: 128000, MaxTokens: 16384}},
				ExposedModels: []string{"old"},
			},
		},
	}
	cfg.Settings.Proxy.Host = "127.0.0.1"
	cfg.Settings.Proxy.Port = 43112

	proposed := BuildProposedGatewayEntry(cfg)
	models, _ := proposed["models"].([]interface{})
	ids := map[string]bool{}
	for _, m := range models {
		ids[m.(map[string]interface{})["id"].(string)] = true
	}
	for _, want := range []string{"sup/main/m1", "sup/bk/m1", "legacy/old"} {
		if !ids[want] {
			t.Fatalf("proposed ids = %v, want %q", ids, want)
		}
	}
	if len(ids) != 3 {
		t.Fatalf("proposed ids = %v, want exactly 3", ids)
	}
}
