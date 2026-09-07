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
		Profiles: map[string]config.ProviderProfile{
			"oc": {
				Upstreams: []config.Upstream{{
					Name: strptr("chat"), API: "openai-responses",
					Models: []config.ModelEntry{{
						ID: "mimo-v2.5", ContextWindow: 1048576, MaxTokens: 131072,
						Name: strptr("MiMo-V2.5"), Cost: &config.ModelCost{Input: 0.4, Output: 2, CacheRead: 0.08},
						Input: []string{"text", "image"}, Reasoning: boolptr(true),
					}}, ExposedModels: []string{"mimo-v2.5"},
				}},
			},
		},
	}
	cfg.Settings.Proxy.Host = "127.0.0.1"
	cfg.Settings.Proxy.Port = 43112

	proposed := BuildProposedGatewayEntry(cfg)
	provs, _ := proposed["providers"].(map[string]interface{})
	if provs == nil {
		t.Fatalf("proposed missing providers")
	}
	// legacy single -> oc
	entry, _ := provs["oc/chat"].(map[string]interface{})
	if entry == nil {
		t.Fatalf("providers missing oc/chat: %v", provs)
	}
	models, _ := entry["models"].([]interface{})
	if len(models) != 1 {
		t.Fatalf("proposed models len = %d, want 1", len(models))
	}
	costJSON, _ := json.Marshal(models[0].(map[string]interface{})["cost"])
	if !strings.Contains(string(costJSON), `"cacheWrite"`) {
		t.Fatalf("proposed cost omits cacheWrite: %s (want explicit cacheWrite:0)", string(costJSON))
	}
	// id must be bare
	if id, _ := models[0].(map[string]interface{})["id"].(string); id != "mimo-v2.5" {
		t.Fatalf("model id = %q, want bare mimo-v2.5", id)
	}
}

func TestProposedOpenCodeProviderEnablesSessionAffinity(t *testing.T) {
	cfg := config.PiSwitchConfig{
		Profiles: map[string]config.ProviderProfile{
			"oc": {
				Upstreams: []config.Upstream{{
					Name:          strptr("chat"),
					API:           "openai-completions",
					BaseURL:       "https://opencode.ai/zen/go/v1",
					Models:        []config.ModelEntry{{ID: "new-model", ContextWindow: 128000, MaxTokens: 16384}},
					ExposedModels: []string{"new-model"},
				}},
			},
		},
	}

	proposed := BuildProposedGatewayEntry(cfg)
	providers := proposed["providers"].(map[string]interface{})
	provider := providers["oc/chat"].(map[string]interface{})
	compat, ok := provider["compat"].(map[string]interface{})
	if !ok || compat["sendSessionAffinityHeaders"] != true {
		t.Fatalf("opencode provider compat = %#v, want sendSessionAffinityHeaders=true", provider["compat"])
	}
}

// 端到端收敛：current 为“已发布一次后的落盘形态”（cost 含 cacheWrite:0），
// 经 MergeGatewayExtra 归一化后 ComputePendingCount 必须为 0。
func TestPreviewPendingConvergesAfterPublish(t *testing.T) {
	cfg := config.PiSwitchConfig{
		Profiles: map[string]config.ProviderProfile{
			"oc": {
				Upstreams: []config.Upstream{{
					Name: strptr("chat"), API: "openai-responses",
					Models: []config.ModelEntry{{
						ID: "mimo-v2.5", ContextWindow: 1048576, MaxTokens: 131072,
						Name: strptr("MiMo-V2.5"), Cost: &config.ModelCost{Input: 0.4, Output: 2, CacheRead: 0.08},
						Input: []string{"text", "image"}, Reasoning: boolptr(true),
					}}, ExposedModels: []string{"mimo-v2.5"},
				}},
			},
		},
	}
	cfg.Settings.Proxy.Host = "127.0.0.1"
	cfg.Settings.Proxy.Port = 43112

	proposed := BuildProposedGatewayEntry(cfg)
	// 模拟落盘 current：与 proposed 同构的 providers wrapper，cost 含 cacheWrite:0
	current := map[string]interface{}{
		"providers": map[string]interface{}{
			"oc/chat": map[string]interface{}{
				"api":     "openai-responses",
				"baseUrl": "http://127.0.0.1:43112/v1",
				"apiKey":  "pi-switch-proxy",
				"proxy":   false,
				"models": []interface{}{
					map[string]interface{}{
						"id":            "mimo-v2.5",
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
			},
		},
	}
	merged := MergeGatewayExtra(current, proposed)
	if got := ComputePendingCount(current, merged); got != 0 {
		a, r, c := DiffGateway(current, merged)
		t.Fatalf("pending = %d, want 0 (added=%v removed=%v changed=%v)", got, a, r, c)
	}
}

// S5：分区聚合按渠道分 provider，裸 id。已分区 profile 按渠道逐条聚合，key = supplier/channel 裸 modelId；
// 跨渠道同裸 id 为独立条目；未分区 legacy 也按 supplier 裸 id 落盘。
func TestProposedChannelPrefixedIDs(t *testing.T) {
	cfg := config.PiSwitchConfig{
		Profiles: map[string]config.ProviderProfile{
			"sup": {
				API: "openai-completions",
				Upstreams: []config.Upstream{
					{
						Name:          strptr("main"),
						API:           "openai-completions",
						Models:        []config.ModelEntry{{ID: "m1", ContextWindow: 100, MaxTokens: 10}},
						ExposedModels: []string{"m1"},
					},
					{
						Name:          strptr("bk"),
						API:           "openai-responses",
						Models:        []config.ModelEntry{{ID: "m1", ContextWindow: 200, MaxTokens: 20}},
						ExposedModels: []string{"m1"},
					},
				},
			},
			"legacy": {
				API:       "openai-completions",
				Upstreams: []config.Upstream{{Name: strptr("main"), API: "openai-completions", Models: []config.ModelEntry{{ID: "old", ContextWindow: 128000, MaxTokens: 16384}}, ExposedModels: []string{"old"}}},
			},
		},
	}
	cfg.Settings.Proxy.Host = "127.0.0.1"
	cfg.Settings.Proxy.Port = 43112

	proposed := BuildProposedGatewayEntry(cfg)
	provs, _ := proposed["providers"].(map[string]interface{})
	if provs == nil {
		t.Fatalf("providers nil")
	}
	// every channel always has its own provider key
	for _, want := range []string{"sup/main", "sup/bk", "legacy/main"} {
		if _, ok := provs[want]; !ok {
			t.Fatalf("providers missing %q: %v", want, provs)
		}
	}
	if _, ok := provs["sup"]; ok {
		t.Fatalf("should not have short sup key")
	}
	// check bare ids
	checkBare := func(key, wantID string) {
		entry, _ := provs[key].(map[string]interface{})
		models, _ := entry["models"].([]interface{})
		if len(models) != 1 {
			t.Fatalf("%s models len %d want 1", key, len(models))
		}
		id, _ := models[0].(map[string]interface{})["id"].(string)
		if id != wantID {
			t.Fatalf("%s id = %q want bare %q", key, id, wantID)
		}
		if strings.Contains(id, "/") {
			t.Fatalf("%s id %q should be bare", key, id)
		}
	}
	checkBare("sup/main", "m1")
	checkBare("sup/bk", "m1")
	checkBare("legacy/main", "old")
	// check per-channel api
	if e, _ := provs["sup/main"].(map[string]interface{}); e["api"] != "openai-completions" {
		t.Fatalf("sup/main api %v want openai-completions", e["api"])
	}
	if e, _ := provs["sup/bk"].(map[string]interface{}); e["api"] != "openai-responses" {
		t.Fatalf("sup/bk api %v want openai-responses", e["api"])
	}
	if len(provs) != 3 {
		t.Fatalf("providers len %d want 3", len(provs))
	}
}
