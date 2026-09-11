package profile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/protocol"
)

func isolate(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PI_SWITCH_CONFIG", path)
	return path
}

func provider() config.ProviderProfile {
	return config.ProviderProfile{
		API:           "openai-completions",
		ResponsesMode: "auto",
		BaseURL:       "https://example.test/v1",
		Upstreams: []config.Upstream{{
			Name:    strptr("main"),
			API:     "openai-completions",
			BaseURL: "https://example.test/v1",
			Models:  []config.ModelEntry{{ID: "m1", ContextWindow: 100, MaxTokens: 10}},
		}},
	}
}

func TestCreateProfilePersistsAndRejectsDuplicates(t *testing.T) {
	path := isolate(t)
	if err := CreateProfile("p", provider()); err != nil {
		t.Fatalf("create: %v", err)
	}
	cfg, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Profiles["p"]; !ok {
		t.Fatalf("profile not persisted: %+v", cfg.Profiles)
	}
	if err := CreateProfile("p", provider()); !errors.Is(err, ErrProfileExists) {
		t.Fatalf("duplicate profile = %v, want ErrProfileExists (a plain errors.New would let callers fall back to string matching)", err)
	}
}

// ValidateProfile is the one sequence both the create path (CreateProfile) and the
// overwriting PUT path run, so it must surface all three rule classes with the order
// the surfaces report them in.
func TestValidateProfileCoversAllThreeRuleClasses(t *testing.T) {
	if err := ValidateProfile(provider()); err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}

	mode := provider()
	mode.ResponsesMode = "passthrough"
	if err := ValidateProfile(mode); err == nil || !strings.Contains(err.Error(), "responsesMode") {
		t.Fatalf("responsesMode rule missing: %v", err)
	}

	shape := provider()
	shape.Upstreams[0].BaseURL = "ftp://example.test"
	if err := ValidateProfile(shape); err == nil || !strings.Contains(err.Error(), "upstreams baseUrl") {
		t.Fatalf("shape rule missing: %v", err)
	}

	tooManyRetries := config.MaxRequestRetry + 1
	retry := provider()
	retry.RequestRetry = &tooManyRetries
	if err := ValidateProfile(retry); err == nil || !strings.Contains(err.Error(), "requestRetry") {
		t.Fatalf("retry rule missing: %v", err)
	}
}

// A flat profile (no channels) *is* the effective pair, so the create path must
// judge it by the same capability rule the other doors apply — protocol.CanProxy
// through config.ValidateEffectiveChannelAPI: unknown api rejected,
// known-but-unproxyable rejected, proxyable accepted. That rule used to be
// reached only via ProfileIssues' channel loop (config.ValidateUpstreamAPI), so
// an absent `upstreams` meant no verdict at all on this door — a flat google
// profile was stored even though every request through it fails.
func TestCreateProfileFlatProfileObeysCapabilityContract(t *testing.T) {
	isolate(t)
	flat := func(api string) config.ProviderProfile {
		return config.ProviderProfile{
			API:           api,
			ResponsesMode: "auto",
			BaseURL:       "https://example.test/v1",
			APIKey:        "k",
		}
	}

	rejected := []struct {
		api  string
		want string
	}{
		{protocol.GoogleGenerativeAI, "not currently proxy-supported"},
		{"unknown-api", "unsupported api"},
	}
	for _, tc := range rejected {
		err := CreateProfile("flat-"+tc.api, flat(tc.api))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("flat %s = %v, want an error containing %q", tc.api, err, tc.want)
		}
	}

	if err := CreateProfile("flat-ok", flat(protocol.OpenAIResponses)); err != nil {
		t.Fatalf("flat proxyable api rejected: %v", err)
	}
}

// DuplicateProfile 逐字复制既有 profile，但复制不能成为绕过 capability 的途径：判定与整文件门
// 共用 config.ValidateResolvedCapability，所以副本不可能是那道门会拒绝的配置——flat 源与 channel
// 源一视同仁。这些源只可能来自遗留配置或手工编辑（三道授权门已经拒收这些形状），所以用直接写盘
// 来构造。shape/retry 仍然不判：带这些问题的源必须还能「复制一份再改」。
func TestDuplicateProfileRefusesUnproxyableSource(t *testing.T) {
	path := isolate(t)
	write := func(profile string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(`{"version":2,"profiles":{"legacy":`+profile+`}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}
	channel := func(channelAPI, channelBaseURL string) string {
		return `{"api":"openai-completions","baseUrl":"https://example.test/v1","apiKey":"k",` +
			`"upstreams":[{"name":"main","api":"` + channelAPI + `","baseUrl":"` + channelBaseURL + `","apiKey":"k","models":[{"id":"m1"}]}]}`
	}

	write(`{"api":"` + protocol.GoogleGenerativeAI + `","baseUrl":"https://example.test/v1","apiKey":"k"}`)
	if err := DuplicateProfile("legacy", "copy"); err == nil || !strings.Contains(err.Error(), "not currently proxy-supported") {
		t.Fatalf("flat unproxyable source = %v, want a proxy-capability error", err)
	}

	write(`{"api":"unknown-api","baseUrl":"https://example.test/v1","apiKey":"k"}`)
	if err := DuplicateProfile("legacy", "copy"); err == nil || !strings.Contains(err.Error(), "unsupported api") {
		t.Fatalf("flat unknown source = %v, want an unsupported-api error", err)
	}

	// channel 型源走同一条判定：点名到具体的 channel，而不是只报一个 api。
	write(channel(protocol.GoogleGenerativeAI, "https://example.test/v1"))
	err := DuplicateProfile("legacy", "copy")
	if err == nil || !strings.Contains(err.Error(), "upstreams[0]") || !strings.Contains(err.Error(), "not currently proxy-supported") {
		t.Fatalf("channel unproxyable source = %v, want an upstreams[0] proxy-capability error", err)
	}

	write(channel("unknown-api", "https://example.test/v1"))
	if err := DuplicateProfile("legacy", "copy"); err == nil || !strings.Contains(err.Error(), "unsupported api") {
		t.Fatalf("channel unknown source = %v, want an unsupported-api error", err)
	}

	// shape 不归 duplicate 管：磁盘上 baseUrl 坏的源（flat 与 channel 两种）仍要能复制一份再改，
	// 否则这条修复路径会被 shape 问题一起挡掉。
	write(`{"api":"openai-completions","baseUrl":"ftp://example.test","apiKey":"k"}`)
	if err := DuplicateProfile("legacy", "copy"); err != nil {
		t.Fatalf("a flat source with a shape problem must still be duplicable: %v", err)
	}

	write(channel("openai-completions", "ftp://example.test"))
	if err := DuplicateProfile("legacy", "copy2"); err != nil {
		t.Fatalf("a channel source with a shape problem must still be duplicable: %v", err)
	}
}

// DuplicateProfile 判两件事：profile 顶层 api/responsesMode 自洽，随后 resolved capability——
// 与整文件门同一顺序。channel 自带 api 覆盖了顶层组合也不豁免：那条"配置自身必须自洽"的检查
// 是整文件门唯一比 runtime 严的地方（system-contract §2.2 第 4 条），duplicate 不能是唯一放行
// 的门。正例见 TestDuplicateProfileAllowsValidTopLevelModeWithChannelOverride。
func TestDuplicateProfileRejectsInvalidTopLevelResponsesModeEvenWhenChannelOverridesIt(t *testing.T) {
	path := isolate(t)
	// 顶层 openai-completions + passthrough 不可执行；channel 的 openai-responses +
	// passthrough 本身有效，所以这一格只有顶层检查能挡住。
	source := `{"api":"openai-completions","responsesMode":"passthrough","baseUrl":"https://example.test/v1","apiKey":"k",` +
		`"upstreams":[{"name":"main","api":"openai-responses","responsesMode":"passthrough","baseUrl":"https://example.test/v1","apiKey":"k","models":[{"id":"m1"}]}]}`
	if err := os.WriteFile(path, []byte(`{"version":2,"profiles":{"legacy":`+source+`}}`), 0644); err != nil {
		t.Fatal(err)
	}

	err := DuplicateProfile("legacy", "copy")
	if err == nil || !strings.Contains(err.Error(), "passthrough") {
		t.Fatalf("invalid top-level pair = %v, want a responsesMode error", err)
	}
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(raw), `"copy"`) {
		t.Fatalf("被拒绝的复制仍然落盘：%s", raw)
	}
}

// 防止修复时把"channel 覆盖顶层组合"本身当成错误：顶层 auto 合法、channel 用
// openai-responses + passthrough 覆盖它，是合法配置（runtime 就按 channel 走），复制必须成功。
func TestDuplicateProfileAllowsValidTopLevelModeWithChannelOverride(t *testing.T) {
	path := isolate(t)
	source := `{"api":"openai-completions","responsesMode":"auto","baseUrl":"https://example.test/v1","apiKey":"k",` +
		`"upstreams":[{"name":"main","api":"openai-responses","responsesMode":"passthrough","baseUrl":"https://example.test/v1","apiKey":"k","models":[{"id":"m1"}]}]}`
	if err := os.WriteFile(path, []byte(`{"version":2,"profiles":{"legacy":`+source+`}}`), 0644); err != nil {
		t.Fatal(err)
	}

	if err := DuplicateProfile("legacy", "copy"); err != nil {
		t.Fatalf("合法的 channel override 被拒绝：%v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"copy"`) {
		t.Fatalf("副本没有落盘：%s", raw)
	}
}

func TestDuplicateProfileErrorKinds(t *testing.T) {
	isolate(t)
	if err := CreateProfile("p", provider()); err != nil {
		t.Fatal(err)
	}
	if err := DuplicateProfile("p", ""); !errors.Is(err, ErrTargetNameRequired) {
		t.Fatalf("empty target = %v, want ErrTargetNameRequired", err)
	}
	if err := DuplicateProfile("missing", "q"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("missing source = %v, want ErrProfileNotFound", err)
	}
	if err := DuplicateProfile("p", "q"); err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if err := DuplicateProfile("q", "q"); !errors.Is(err, ErrProfileExists) {
		t.Fatalf("existing target = %v, want ErrProfileExists", err)
	}
}

func TestSetExposedModelsValidatesChannelAndPool(t *testing.T) {
	isolate(t)
	if err := CreateProfile("p", provider()); err != nil {
		t.Fatal(err)
	}
	if err := SetExposedModels("p", "nope", []string{"m1"}); !errors.Is(err, ErrUnknownChannel) {
		t.Fatalf("unknown channel = %v, want ErrUnknownChannel", err)
	}
	if err := SetExposedModels("p", "main", []string{"not-in-pool"}); err == nil {
		t.Fatal("exposing a model outside the channel pool was accepted")
	}
	if err := SetExposedModels("p", "main", []string{"m1"}); err != nil {
		t.Fatalf("expose: %v", err)
	}
}

func TestEnsureMutationChannelUpgradesLegacyUnnamedUpstream(t *testing.T) {
	prof := config.ProviderProfile{
		API:     "openai-completions",
		BaseURL: "https://legacy.test/v1",
		APIKey:  "k",
		Upstreams: []config.Upstream{{
			API:     "openai-completions",
			BaseURL: "https://legacy.test/v1",
			APIKey:  "k",
		}},
	}
	idx := EnsureMutationChannel(&prof, "main")
	if idx != 0 || prof.ChannelName(0) != "main" {
		t.Fatalf("legacy upgrade failed: idx=%d prof=%+v", idx, prof.Upstreams)
	}
	if got := EnsureMutationChannel(&prof, "other"); got != -1 {
		t.Fatalf("unknown channel = %d, want -1", got)
	}
}

func strptr(s string) *string { return &s }
