package config

import (
	"context"
	"fmt"

	"github.com/heihei0299/pi-switch/internal/adapters/outbound/fileconfig"
	domainconfig "github.com/heihei0299/pi-switch/internal/domain/config"
	"github.com/heihei0299/pi-switch/internal/domain/provider"
	"github.com/heihei0299/pi-switch/internal/protocol"
)

type ModelCost = provider.ModelCost
type ModelEntry = provider.ModelEntry
type RequestScopedError = provider.RequestScopedError
type Upstream = provider.Upstream
type ProviderProfile = provider.Profile

// ValidateEffectiveChannelAPI checks the api/mode pair a channel will actually be
// called with — effective api, effective mode — which is exactly what the request
// path resolves before protocol.PlanRequest (narrowToChannel over
// ResolvedUpstreams). It does not demand that the channel declare its own api, but a
// pair with no api anywhere is not "nothing to judge": every request through that
// channel fails, so it is an error like any other incompatible pair.
//
// A known-but-unproxyable api is rejected too: protocol.CanProxy is the one
// capability source, protocol.upstreamFormat refuses every request for such an api
// (google-generative-ai today), and the write doors therefore refuse to store a
// profile that cannot run instead of surfacing the failure at request time.
func ValidateEffectiveChannelAPI(u Upstream, profile ProviderProfile) error {
	api := u.EffectiveAPI(profile.API)
	if api == "" {
		return fmt.Errorf("api is required (neither the profile nor the channel declares one)")
	}
	if !protocol.IsKnown(api) {
		return fmt.Errorf("unsupported api %s", api)
	}
	if !protocol.CanProxy(api) {
		return fmt.Errorf("api %s is not currently proxy-supported", api)
	}
	return protocol.ValidateResponsesMode(api, u.EffectiveResponsesMode(profile.ResponsesMode))
}

// ValidateEffectiveFlatAPI judges the api/mode pair of a profile when nothing resolves to
// a channel, so the profile's own api is the effective one — exactly what a synthesized
// legacy channel would have been judged with. Callers use it once they know the profile
// has no resolved channel (ValidateProfile's flat branch, ValidateResolvedCapability, the
// advisory door) instead of passing `Upstream{}` and guessing what it stands for.
func ValidateEffectiveFlatAPI(profile ProviderProfile) error {
	return ValidateEffectiveChannelAPI(Upstream{}, profile)
}

// ValidateResolvedCapability judges a profile's api/mode capability using the same
// effective-upstream fallback semantics used by the runtime helpers. It checks every
// ResolvedUpstreams() entry and, when none exists but the profile declares an api,
// checks the profile-level api/mode pair. A profile that declares no api anywhere has
// no pair to judge and passes.
//
// It is the capability rule the whole-file config door applies to a profile being stored
// (that door's profile-level mode check runs first), and the rule DuplicateProfile
// applies before copying one, so a copy can never be a profile that door would refuse.
// Shape, model-name and retry rules are deliberately not part of it: those belong to the
// authoring doors.
func ValidateResolvedCapability(profile ProviderProfile) error {
	resolved := profile.ResolvedUpstreams()
	for idx, u := range resolved {
		if err := ValidateEffectiveChannelAPI(u, profile); err != nil {
			return fmt.Errorf("upstreams[%d]: %w", idx, err)
		}
	}
	if len(resolved) == 0 && profile.API != "" {
		return ValidateEffectiveFlatAPI(profile)
	}
	return nil
}

// ValidateUpstreamAPI checks one channel of a profile that is being authored, so it
// additionally requires the channel to name its own api. It reports exactly what the
// effective rule reports: an explicit per-channel api with an incompatible mode is
// the combination protocol.PlanRequest rejects at request time.
func ValidateUpstreamAPI(u Upstream, profile ProviderProfile) error {
	if u.API == "" {
		return fmt.Errorf("api is required for each upstream")
	}
	return ValidateEffectiveChannelAPI(u, profile)
}

func validateUpstreamAPI(u Upstream, profile ProviderProfile) error {
	return ValidateUpstreamAPI(u, profile)
}

type CircuitBreakerSettings = domainconfig.CircuitBreakerSettings
type Settings = domainconfig.Settings
type PiSwitchConfig = domainconfig.PiSwitchConfig

func DefaultConfig() PiSwitchConfig {
	pp := "test-provider"
	channel := "main"
	return PiSwitchConfig{
		Version: 2,
		Current: &pp,
		Profiles: map[string]ProviderProfile{
			"test-provider": {
				API:           protocol.OpenAIChat,
				ResponsesMode: "auto",
				BaseURL:       "https://api.openai.com/v1",
				APIKey:        "sk-prototype-not-real",
				Upstreams: []Upstream{{
					Name:          &channel,
					API:           protocol.OpenAIChat,
					BaseURL:       "https://api.openai.com/v1",
					APIKey:        "sk-prototype-not-real",
					Models:        []ModelEntry{{ID: "gpt-4o-mini", ContextWindow: 128000, MaxTokens: 16384, Cost: &ModelCost{Input: 0.15, Output: 0.6, CacheRead: 0.075}}},
					ExposedModels: []string{"gpt-4o-mini"},
				}},
			},
		},
		Settings: Settings{
			WriteMode:          "gateway",
			ConversationSource: "sessionScan",
			Proxy: struct {
				Host                          string                 `json:"host"`
				Port                          int                    `json:"port"`
				UserAgent                     *string                `json:"userAgent,omitempty"`
				CircuitBreaker                CircuitBreakerSettings `json:"circuitBreaker"`
				RequestRetry                  *int                   `json:"requestRetry,omitempty"`
				MaxRetryCredentials           int                    `json:"maxRetryCredentials,omitempty"`
				MaxRetryInterval              *int                   `json:"maxRetryInterval,omitempty"`
				DisableCooling                *bool                  `json:"disableCooling,omitempty"`
				TransientErrorCooldownSeconds *int                   `json:"transientErrorCooldownSeconds,omitempty"`
				RequestScopedErrors           []RequestScopedError   `json:"requestScopedErrors,omitempty"`
			}{Host: "127.0.0.1", Port: 43112, CircuitBreaker: CircuitBreakerSettings{Enabled: true, FailureThreshold: 3, CooldownSeconds: 60}},
			Web: struct {
				Host string `json:"host"`
				Port int    `json:"port"`
			}{Host: "127.0.0.1", Port: 43110},
		},
	}
}

func MigratedForSave(cfg PiSwitchConfig) PiSwitchConfig {
	return domainconfig.MigratedForSave(cfg)
}

// ResolvePath preserves the legacy config entry point; fileconfig owns path resolution.
func ResolvePath() string { return fileconfig.ResolvePath() }

// ErrConfigWriteLockBusy preserves the legacy storage error identity.
var ErrConfigWriteLockBusy = fileconfig.ErrConfigWriteLockBusy

// SaveAtPath preserves the legacy config entry point while fileconfig owns persistence.
func SaveAtPath(cfg PiSwitchConfig, path string) error {
	return fileconfig.New(path, DefaultConfig).Save(cfg)
}

func UpdateAtPath(path string, mutate func(*PiSwitchConfig) error) error {
	return fileconfig.New(path, DefaultConfig).UpdateConfig(context.Background(), false, mutate)
}

func ReplaceAtPath(path string, prepare func([]byte) (PiSwitchConfig, error)) error {
	return fileconfig.New(path, DefaultConfig).Replace(context.Background(), prepare)
}

func ParseConfig(b []byte) (PiSwitchConfig, error) {
	return domainconfig.ParseConfig(b, DefaultConfig())
}

func LoadConfigAtPath(path string) (PiSwitchConfig, string, error) {
	return fileconfig.New(path, DefaultConfig).Load()
}

// MergeHeaders preserves the existing config package entry point.
func MergeHeaders(profileHeaders, channelHeaders map[string]string) map[string]string {
	return provider.MergeHeaders(profileHeaders, channelHeaders)
}
