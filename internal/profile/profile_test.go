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

// DuplicateProfile 逐字复制既有 profile，但复制不能成为绕过 capability 的途径：flat profile
// 的 api 就是它的 effective pair，把一条跑不起来的 legacy profile 再复制一份，同样是让 CRUD
// 产出一个必失败配置。这些源只可能来自遗留配置或手工编辑（三个授权门已经拒收这个形状），所以
// 用直接写盘来构造。
func TestDuplicateProfileRefusesUnproxyableFlatSource(t *testing.T) {
	path := isolate(t)
	write := func(profile string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(`{"version":2,"profiles":{"legacy":`+profile+`}}`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	write(`{"api":"` + protocol.GoogleGenerativeAI + `","baseUrl":"https://example.test/v1","apiKey":"k"}`)
	if err := DuplicateProfile("legacy", "copy"); err == nil || !strings.Contains(err.Error(), "proxy") {
		t.Fatalf("flat unproxyable source = %v, want a proxy-capability error", err)
	}

	write(`{"api":"unknown-api","baseUrl":"https://example.test/v1","apiKey":"k"}`)
	if err := DuplicateProfile("legacy", "copy"); err == nil || !strings.Contains(err.Error(), "unsupported api") {
		t.Fatalf("flat unknown source = %v, want an unsupported-api error", err)
	}

	// shape 不归 duplicate 管：磁盘上 baseUrl 坏的 legacy profile 仍要能复制一份再改，
	// 否则这条修复路径会被 shape 问题一起挡掉。
	write(`{"api":"openai-completions","baseUrl":"ftp://example.test","apiKey":"k"}`)
	if err := DuplicateProfile("legacy", "copy"); err != nil {
		t.Fatalf("a flat source with a shape problem must still be duplicable: %v", err)
	}

	// 可代理的 flat 源照常复制。
	write(`{"api":"openai-completions","baseUrl":"https://example.test/v1","apiKey":"k"}`)
	if err := DuplicateProfile("legacy", "copy2"); err != nil {
		t.Fatalf("proxyable flat source: %v", err)
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
