package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	entry, _ := provs["pi-switch-res"].(map[string]interface{})
	if entry == nil {
		t.Fatalf("providers missing pi-switch-res: %v", provs)
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

func TestProposedOpenCodeModelEnablesSessionAffinity(t *testing.T) {
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
	provider := providers["pi-switch-chat"].(map[string]interface{})
	models := provider["models"].([]interface{})
	model := models[0].(map[string]interface{})
	compat, ok := model["compat"].(map[string]interface{})
	if !ok || compat["sendSessionAffinityHeaders"] != true {
		t.Fatalf("opencode model compat = %#v, want sendSessionAffinityHeaders=true", model["compat"])
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
			"pi-switch-res": map[string]interface{}{
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

func TestCanonicalGatewayPlanUnifiesViews(t *testing.T) {
	channel := "main"
	cfg := config.PiSwitchConfig{Profiles: map[string]config.ProviderProfile{
		"sup": {Upstreams: []config.Upstream{{Name: &channel, API: "openai-completions", Models: []config.ModelEntry{{ID: "m1", ContextWindow: 100, MaxTokens: 10}}, ExposedModels: []string{"m1"}}}},
	}}
	cfg.Settings.Proxy.Host = "127.0.0.1"
	cfg.Settings.Proxy.Port = 43112
	current := map[string]interface{}{"providers": map[string]interface{}{
		gatewayChatProvider: map[string]interface{}{
			"api": "openai-completions", "baseUrl": "http://127.0.0.1:43112/v1", "apiKey": "pi-switch-proxy", "proxy": false,
			"models": []interface{}{map[string]interface{}{"id": "m1", "contextWindow": float64(100), "maxTokens": float64(10)}},
		},
		"third-party": map[string]interface{}{"api": "openai-completions", "baseUrl": "https://third.example/v1", "apiKey": "third-key", "models": []interface{}{map[string]interface{}{"id": "third-model"}}},
	}}
	plan := BuildCanonicalGatewayPlan(cfg, current, nil)
	if len(plan.Conflicts) != 0 || plan.PendingCount != 0 {
		t.Fatalf("canonical plan = conflicts=%v pending=%d proposed=%v", plan.Conflicts, plan.PendingCount, plan.Proposed)
	}
	providers := plan.Proposed["providers"].(map[string]interface{})
	if _, ok := providers["third-party"]; !ok {
		t.Fatalf("canonical plan dropped third-party provider: %v", providers)
	}
	if len(plan.Groups) != 1 || plan.Groups[0].Models[0].Status != "published" {
		t.Fatalf("canonical groups = %#v", plan.Groups)
	}
	if len(plan.Added) != 0 || len(plan.Removed) != 0 || len(plan.Changed) != 0 {
		t.Fatalf("canonical diff = added=%v removed=%v changed=%v", plan.Added, plan.Removed, plan.Changed)
	}
}

func TestCanonicalPlanPreservesThirdPartyEntryFromCurrent(t *testing.T) {
	cfg := config.PiSwitchConfig{}
	current := map[string]interface{}{"providers": map[string]interface{}{
		"third-party": map[string]interface{}{"api": "openai-completions", "apiKey": "original", "models": []interface{}{map[string]interface{}{"id": "m"}}, "custom": "keep"},
	}}
	edited := map[string]interface{}{"providers": map[string]interface{}{
		"third-party": map[string]interface{}{"api": "openai-completions", "apiKey": "tampered", "models": []interface{}{map[string]interface{}{"id": "changed"}}},
	}}
	before, _ := json.Marshal(edited)
	plan := BuildCanonicalGatewayPlan(cfg, current, edited)
	providers := plan.Proposed["providers"].(map[string]interface{})
	after, _ := json.Marshal(edited)
	if string(after) != string(before) {
		t.Fatalf("canonical plan mutated edited draft: before=%s after=%s", before, after)
	}
	if len(plan.Conflicts) != 1 || !strings.Contains(plan.Conflicts[0], "third-party provider third-party is read-only") {
		t.Fatalf("third-party edit conflict = %v", plan.Conflicts)
	}
	got := providers["third-party"].(map[string]interface{})
	if got["apiKey"] != "original" || got["custom"] != "keep" {
		t.Fatalf("third-party entry changed: %#v", got)
	}
}

// Fixed gateway providers keep model ids bare while separating API contracts.
func TestProposedGatewayUsesBareIDsAcrossFixedProviders(t *testing.T) {
	cfg := config.PiSwitchConfig{
		Profiles: map[string]config.ProviderProfile{
			"sup": {
				Upstreams: []config.Upstream{
					{Name: strptr("main"), API: "openai-completions", Models: []config.ModelEntry{{ID: "m1", ContextWindow: 100, MaxTokens: 10}}, ExposedModels: []string{"m1"}},
					{Name: strptr("bk"), API: "openai-responses", Models: []config.ModelEntry{{ID: "r1", ContextWindow: 200, MaxTokens: 20}}, ExposedModels: []string{"r1"}},
				},
			},
			"legacy": {
				Upstreams: []config.Upstream{{Name: strptr("main"), API: "openai-completions", Models: []config.ModelEntry{{ID: "old", ContextWindow: 128000, MaxTokens: 16384}}, ExposedModels: []string{"old"}}},
			},
		},
	}
	cfg.Settings.Proxy.Host = "127.0.0.1"
	cfg.Settings.Proxy.Port = 43112

	proposed := BuildProposedGatewayEntry(cfg)
	provs := proposed["providers"].(map[string]interface{})
	if len(provs) != 2 {
		t.Fatalf("providers len = %d, want 2: %v", len(provs), provs)
	}
	res := provs["pi-switch-res"].(map[string]interface{})
	chat := provs["pi-switch-chat"].(map[string]interface{})
	if res["api"] != "openai-responses" || chat["api"] != "openai-completions" {
		t.Fatalf("provider APIs = responses:%v chat:%v", res["api"], chat["api"])
	}
	modelIDs := func(provider map[string]interface{}) []string {
		raw := provider["models"].([]interface{})
		ids := make([]string, 0, len(raw))
		for _, item := range raw {
			id := item.(map[string]interface{})["id"].(string)
			if strings.Contains(id, "/") {
				t.Fatalf("model id %q contains a route prefix", id)
			}
			ids = append(ids, id)
		}
		return ids
	}
	if got := strings.Join(modelIDs(res), ","); got != "r1" {
		t.Fatalf("Responses ids = %q, want r1", got)
	}
	if got := strings.Join(modelIDs(chat), ","); got != "m1,old" {
		t.Fatalf("Chat ids = %q, want m1,old", got)
	}
}

func TestPublishPreservesThirdPartyProviders(t *testing.T) {
	modelsPath := filepath.Join(t.TempDir(), "models.json")
	t.Setenv("PI_SWITCH_MODELS", modelsPath)

	thirdParty := map[string]interface{}{
		"api":     "openai-responses",
		"baseUrl": "https://third-party.example/v1",
		"apiKey":  "third-party-key",
		"headers": map[string]interface{}{"X-Third-Party": "keep-me"},
		"models":  []interface{}{map[string]interface{}{"id": "third-model", "contextWindow": float64(200000)}},
		"custom":  map[string]interface{}{"ownedBy": "someone-else"},
	}
	initial := map[string]interface{}{
		"providers": map[string]interface{}{
			"third-party": thirdParty,
			"pi-switch-chat": map[string]interface{}{
				"api": "openai-completions", "baseUrl": "http://127.0.0.1:43112/v1",
				"apiKey": "pi-switch-proxy", "models": []interface{}{}, "proxy": false,
			},
		},
	}
	data, err := json.Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modelsPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.PiSwitchConfig{Profiles: map[string]config.ProviderProfile{
		"oc": {Upstreams: []config.Upstream{{Name: strptr("chat"), API: "openai-completions"}}},
	}}
	edited := map[string]interface{}{
		"providers": map[string]interface{}{
			"pi-switch-chat": map[string]interface{}{
				"api": "openai-completions", "baseUrl": "http://127.0.0.1:43112/v1",
				"apiKey": "pi-switch-proxy", "models": []interface{}{map[string]interface{}{"id": "new-model"}}, "proxy": false,
			},
		},
	}
	if err := Publish(cfg, edited); err != nil {
		t.Fatal(err)
	}

	result, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}
	firstPublish := string(result)
	if err := Publish(cfg, edited); err != nil {
		t.Fatal(err)
	}
	secondPublish, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(secondPublish) != firstPublish {
		t.Fatalf("second publish changed models.json:\nfirst=%s\nsecond=%s", firstPublish, secondPublish)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(result, &got); err != nil {
		t.Fatal(err)
	}
	providers := got["providers"].(map[string]interface{})
	gotThird := providers["third-party"]
	wantJSON, _ := json.Marshal(thirdParty)
	gotJSON, _ := json.Marshal(gotThird)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("third-party provider changed or disappeared: got %s want %s", gotJSON, wantJSON)
	}
	if _, ok := providers["pi-switch-chat"]; !ok {
		t.Fatal("published pi-switch provider missing")
	}
}

func TestProposedGatewayAggregatesFixedProviders(t *testing.T) {
	cfg := config.PiSwitchConfig{
		Profiles: map[string]config.ProviderProfile{
			"oc": {
				Upstreams: []config.Upstream{
					{Name: strptr("chat"), API: "openai-completions", BaseURL: "https://opencode.ai/zen/go/v1", Models: []config.ModelEntry{{ID: "z-chat", ContextWindow: 128000, MaxTokens: 16000}, {ID: "a-chat", ContextWindow: 128000, MaxTokens: 16000}}, ExposedModels: []string{"z-chat", "a-chat"}},
					{Name: strptr("responses"), API: "openai-responses", BaseURL: "https://opencode.ai/zen/go/v1", Models: []config.ModelEntry{{ID: "z-res", ContextWindow: 256000, MaxTokens: 32000}, {ID: "a-res", ContextWindow: 256000, MaxTokens: 32000}}, ExposedModels: []string{"z-res", "a-res"}},
				},
			},
			"deepseek": {
				Upstreams: []config.Upstream{{Name: strptr("main"), API: "openai-completions", BaseURL: "https://api.deepseek.com", Models: []config.ModelEntry{{ID: "m-chat", ContextWindow: 64000, MaxTokens: 8000}}, ExposedModels: []string{"m-chat"}}},
			},
		},
	}
	cfg.Settings.Proxy.Host = "127.0.0.1"
	cfg.Settings.Proxy.Port = 43112

	proposed := BuildProposedGatewayEntry(cfg)
	providers, ok := proposed["providers"].(map[string]interface{})
	if !ok {
		t.Fatalf("proposed providers = %#v", proposed["providers"])
	}
	if len(providers) != 2 {
		t.Fatalf("provider count = %d, want 2: %#v", len(providers), providers)
	}
	for _, key := range []string{"oc/chat", "oc/responses", "deepseek/main"} {
		if _, exists := providers[key]; exists {
			t.Fatalf("legacy provider %q still present", key)
		}
	}
	res, ok := providers["pi-switch-res"].(map[string]interface{})
	if !ok || res["api"] != "openai-responses" {
		t.Fatalf("pi-switch-res = %#v", providers["pi-switch-res"])
	}
	chat, ok := providers["pi-switch-chat"].(map[string]interface{})
	if !ok || chat["api"] != "openai-completions" {
		t.Fatalf("pi-switch-chat = %#v", providers["pi-switch-chat"])
	}
	modelIDs := func(provider map[string]interface{}) string {
		raw, _ := provider["models"].([]interface{})
		ids := make([]string, 0, len(raw))
		for _, item := range raw {
			id, _ := item.(map[string]interface{})["id"].(string)
			ids = append(ids, id)
		}
		return strings.Join(ids, ",")
	}
	if got := modelIDs(res); got != "a-res,z-res" {
		t.Fatalf("Responses model order = %q, want a-res,z-res", got)
	}
	if got := modelIDs(chat); got != "a-chat,m-chat,z-chat" {
		t.Fatalf("Chat model order = %q, want a-chat,m-chat,z-chat", got)
	}
	chatModels := chat["models"].([]interface{})
	for _, raw := range chatModels {
		model := raw.(map[string]interface{})
		if model["id"] == "z-chat" {
			compat, _ := model["compat"].(map[string]interface{})
			if compat["sendSessionAffinityHeaders"] != true {
				t.Fatalf("opencode model compat = %#v", model["compat"])
			}
		}
		if model["id"] == "m-chat" {
			if _, exists := model["compat"]; exists {
				t.Fatalf("ordinary model unexpectedly has compat: %#v", model["compat"])
			}
		}
	}
}

func TestBuildPreviewGroupsMapsSourcesToFixedProviders(t *testing.T) {
	chat := "chat"
	resp := "responses"
	cfg := config.PiSwitchConfig{
		Profiles: map[string]config.ProviderProfile{
			"oc": {Upstreams: []config.Upstream{
				{Name: &chat, API: "openai-completions", ExposedModels: []string{"m-chat"}},
				{Name: &resp, API: "openai-responses", ExposedModels: []string{"m-res"}},
			}},
		},
	}
	current := map[string]interface{}{"providers": map[string]interface{}{
		"pi-switch-chat": map[string]interface{}{"api": "openai-completions", "models": []interface{}{map[string]interface{}{"id": "m-chat"}}},
		"pi-switch-res":  map[string]interface{}{"api": "openai-responses", "models": []interface{}{map[string]interface{}{"id": "m-res"}}},
	}}
	proposed := map[string]interface{}{"providers": map[string]interface{}{
		"pi-switch-chat": map[string]interface{}{"api": "openai-completions", "models": []interface{}{map[string]interface{}{"id": "m-chat"}}},
		"pi-switch-res":  map[string]interface{}{"api": "openai-responses", "models": []interface{}{map[string]interface{}{"id": "m-res"}}},
	}}
	groups, removed := BuildPreviewGroups(cfg, current, proposed)
	if len(removed) != 0 {
		t.Fatalf("removed = %v, want none", removed)
	}
	encoded, err := json.Marshal(groups)
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]interface{}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("groups len = %d, want 2: %s", len(got), encoded)
		if got[0]["gatewayProvider"] != "pi-switch-res" || got[1]["gatewayProvider"] != "pi-switch-chat" {
			t.Fatalf("group provider order = %v, want responses then chat", got)
		}
	}
	for _, want := range []struct{ supplier, channel, provider, model string }{
		{"oc", "chat", "pi-switch-chat", "m-chat"},
		{"oc", "responses", "pi-switch-res", "m-res"},
	} {
		var found map[string]interface{}
		for _, group := range got {
			if group["supplier"] == want.supplier && group["channel"] == want.channel {
				found = group
				break
			}
		}
		if found == nil {
			t.Fatalf("missing group %s/%s: %s", want.supplier, want.channel, encoded)
		}
		if found["gatewayProvider"] != want.provider {
			t.Fatalf("%s/%s gatewayProvider = %v, want %s", want.supplier, want.channel, found["gatewayProvider"], want.provider)
		}
		models := found["models"].([]interface{})
		item := models[0].(map[string]interface{})
		if item["id"] != want.model || item["status"] != "published" {
			t.Fatalf("%s/%s model = %#v, want published %s", want.supplier, want.channel, item, want.model)
		}
	}
}

func TestPublishRejectsInvalidCanonicalPlanWithoutWrite(t *testing.T) {
	modelsPath := filepath.Join(t.TempDir(), "models.json")
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	original := []byte(`{"providers":{"third-party":{"api":"openai-completions","models":[]}}}`)
	if err := os.WriteFile(modelsPath, original, 0644); err != nil {
		t.Fatal(err)
	}
	main, backup := "main", "backup"
	cfg := config.PiSwitchConfig{Profiles: map[string]config.ProviderProfile{
		"alpha": {Upstreams: []config.Upstream{{Name: &main, API: "openai-completions", ExposedModels: []string{"same"}}}},
		"beta":  {Upstreams: []config.Upstream{{Name: &backup, API: "openai-completions", ExposedModels: []string{"same"}}}},
	}}
	edited := map[string]interface{}{"providers": map[string]interface{}{
		gatewayChatProvider: map[string]interface{}{
			"api": "openai-completions", "baseUrl": "http://127.0.0.1:43112/v1", "apiKey": "pi-switch-proxy", "models": []interface{}{map[string]interface{}{"id": "same"}},
		},
	}}
	if err := Publish(cfg, edited); err == nil {
		t.Fatal("Publish accepted duplicate exposed model")
	}
	got, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("models.json changed after rejected publish: %s", got)
	}
}
