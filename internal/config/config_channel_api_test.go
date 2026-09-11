package config

import "testing"

func TestChannelAPI_IsRequired(t *testing.T) {
	prof := ProviderProfile{API: "openai-completions", ResponsesMode: "auto"}
	if err := ValidateUpstreamAPI(Upstream{}, prof); err == nil {
		t.Fatal("expected missing channel api error")
	}
	if err := ValidateUpstreamAPI(Upstream{API: "invalid-api"}, prof); err == nil {
		t.Fatal("expected invalid api error")
	}
	if err := ValidateUpstreamAPI(Upstream{API: "openai-responses"}, prof); err != nil {
		t.Fatalf("valid channel api error: %v", err)
	}
}

// 后续审查 P2：整文件门按 channel 的 effective api/mode 判，而不是要求 channel 自带
// api。effective 语义 = channel 声明优先、否则回退 profile，与运行期一致；因此
// 「channel 未声明 api」不是错误，而「显式声明 api 却配了不兼容 mode」是。
func TestChannelAPI_EffectiveRulesForTheTolerantDoor(t *testing.T) {
	prof := ProviderProfile{API: "openai-completions", ResponsesMode: "auto"}

	// 未声明 api：按 profile api 判 → openai-completions + auto 合法。
	if err := ValidateEffectiveChannelAPI(Upstream{}, prof); err != nil {
		t.Fatalf("channel without its own api = %v, want nil (the profile api is the fallback)", err)
	}
	// 两处都没有 api：运行期 upstreamFormat("") 必失败，所以是错误而不是「无可判定」。
	if err := ValidateEffectiveChannelAPI(Upstream{}, ProviderProfile{}); err == nil {
		t.Fatal("no api anywhere must be rejected (every request through that channel fails)")
	}
	// 显式声明 api 但 mode 不兼容：请求期必被 translator.PlanRequest 拒绝。
	if err := ValidateEffectiveChannelAPI(Upstream{API: "openai-completions"}, ProviderProfile{ResponsesMode: "passthrough"}); err == nil {
		t.Fatal("openai-completions + passthrough must be rejected")
	}
	// channel 自己的 mode 覆盖 profile 的。
	if err := ValidateEffectiveChannelAPI(Upstream{API: "openai-completions", ResponsesMode: "convert"}, ProviderProfile{ResponsesMode: "passthrough"}); err != nil {
		t.Fatalf("a channel mode should win over the profile's: %v", err)
	}
	// 未知 api 照报（消息比 mode 错误更贴切）。
	if err := ValidateEffectiveChannelAPI(Upstream{API: "invalid-api"}, prof); err == nil {
		t.Fatal("unknown api must be rejected")
	}
	// 严格门额外要求 channel 自带 api —— 两者差别只有这一点。
	if err := ValidateUpstreamAPI(Upstream{}, prof); err == nil {
		t.Fatal("ValidateUpstreamAPI must still demand a per-channel api")
	}
}

func TestChannelAPI_RoundTripKeepsIndependentAPIs(t *testing.T) {
	chat := "chat"
	responses := "responses"
	prof := ProviderProfile{Upstreams: []Upstream{
		{Name: &chat, API: "openai-completions"},
		{Name: &responses, API: "openai-responses"},
	}}
	if got := prof.Upstreams[0].API; got != "openai-completions" {
		t.Fatalf("chat api = %q", got)
	}
	if got := prof.Upstreams[1].API; got != "openai-responses" {
		t.Fatalf("responses api = %q", got)
	}
}
