package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/heihei0299/pi-switch/internal/protocol"
)

type ModelCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite,omitempty"`
}

type ModelEntry struct {
	ID            string     `json:"id"`
	ContextWindow uint32     `json:"contextWindow"`
	MaxTokens     uint32     `json:"maxTokens"`
	Name          *string    `json:"name,omitempty"`
	Cost          *ModelCost `json:"cost,omitempty"`
	Input         []string   `json:"input,omitempty"`
	Reasoning     *bool      `json:"reasoning,omitempty"`
}

type RequestScopedError struct {
	Status int      `json:"status"`
	Match  []string `json:"match,omitempty"`
	Action string   `json:"action"`
}

type Upstream struct {
	BaseURL       string            `json:"baseUrl"`
	APIKey        string            `json:"apiKey"`
	Headers       map[string]string `json:"headers,omitempty"`
	Weight        *uint32           `json:"weight,omitempty"`
	Name          *string           `json:"name,omitempty"`
	API           string            `json:"api,omitempty"`
	ResponsesMode string            `json:"responsesMode,omitempty"`
	// RequestRetry overrides the profile/global retry budget for attempts
	// through this channel. Nil or negative inherits; explicit 0 admits round 0 only.
	RequestRetry *int `json:"requestRetry,omitempty"`
	// DisableCooling overrides cooling for this channel when non-nil.
	DisableCooling *bool `json:"disableCooling,omitempty"`
	// Models / ExposedModels are owned by this channel.
	Models        []ModelEntry `json:"models,omitempty"`
	ExposedModels []string     `json:"exposedModels,omitempty"`
}

// UnmarshalJSON rejects `exposedModels: null` at the boundary. encoding/json folds
// null into a nil slice, but null is neither "missing" (legacy migration owns that)
// nor "[]" (explicit zero exposure) — system-contract 2.2. Keeping the rule on the
// field means the JSON name has one owner instead of a second schema walker.
func (u *Upstream) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["exposedModels"]; ok && isJSONNull(v) {
		return fmt.Errorf("exposedModels must not be null")
	}
	type upstreamAlias Upstream
	return json.Unmarshal(data, (*upstreamAlias)(u))
}

func isJSONNull(v json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(v), []byte("null"))
}

func (u Upstream) EffectiveResponsesMode(fallback string) string {
	if u.ResponsesMode != "" {
		return u.ResponsesMode
	}
	return fallback
}

// EffectiveAPI resolves the api a channel really speaks: its own when it declares
// one, otherwise the profile's. The runtime resolves the same way, so anything that
// judges a channel must judge it by this value rather than by u.API alone.
func (u Upstream) EffectiveAPI(fallback string) string {
	if u.API != "" {
		return u.API
	}
	return fallback
}

// ValidateEffectiveChannelAPI checks the api/mode pair a channel will actually be
// called with — effective api, effective mode — which is exactly what the request
// path resolves before translator.PlanRequest (narrowToChannel over
// ResolvedUpstreams). It does not demand that the channel declare its own api, but a
// pair with no api anywhere is not "nothing to judge": every request through that
// channel fails, so it is an error like any other incompatible pair.
//
// A known-but-unproxyable api is rejected too: protocol.CanProxy is the one
// capability source, translator.upstreamFormat refuses every request for such an api
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

// ValidateResolvedCapability is the capability judgement of one whole profile, in the
// shape the runtime resolves it: every channel from ResolvedUpstreams(), and — when
// nothing resolves to a channel — the profile's own api. A profile that declares no api
// anywhere has no pair to judge and passes.
//
// It is the rule the whole-file config door applies to a profile being stored, and the
// rule DuplicateProfile applies before copying one, so a copy can never be a profile that
// door would refuse. Shape, model-name and retry rules are deliberately not part of it:
// those belong to the authoring doors.
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
// the combination translator.PlanRequest rejects at request time.
func ValidateUpstreamAPI(u Upstream, profile ProviderProfile) error {
	if u.API == "" {
		return fmt.Errorf("api is required for each upstream")
	}
	return ValidateEffectiveChannelAPI(u, profile)
}

func validateUpstreamAPI(u Upstream, profile ProviderProfile) error {
	return ValidateUpstreamAPI(u, profile)
}

type ProviderProfile struct {
	API               string                 `json:"api"`
	ResponsesMode     string                 `json:"responsesMode"`
	BaseURL           string                 `json:"baseUrl"`
	APIKey            string                 `json:"apiKey"`
	Upstreams         []Upstream             `json:"upstreams,omitempty"`
	Headers           map[string]string      `json:"headers,omitempty"`
	ModelMap          map[string]interface{} `json:"modelMap,omitempty"`
	UserAgent         *string                `json:"userAgent,omitempty"`
	Preset            *string                `json:"preset,omitempty"`
	ModelsDevProvider *string                `json:"modelsDevProvider,omitempty"`
	Proxy             *bool                  `json:"proxy,omitempty"`
	// RequestRetry overrides the global retry budget. Nil or negative inherits;
	// explicit 0 admits round 0 only (no additional rounds).
	RequestRetry *int `json:"requestRetry,omitempty"`
	// DisableCooling overrides global cooling when non-nil.
	DisableCooling *bool `json:"disableCooling,omitempty"`
	// RequestScopedErrors classifies upstream errors for this profile.
	// Non-empty overrides the global rules.
	RequestScopedErrors []RequestScopedError `json:"requestScopedErrors,omitempty"`
}

// presetModelsDevProviders maps the built-in presets to their models.dev provider
// id. It is the only place that translation lives, so profile metadata lookup and
// model enrichment cannot drift apart.
var presetModelsDevProviders = map[string]string{
	"openai": "openai", "anthropic": "anthropic", "google": "google",
	"deepseek": "deepseek", "xai": "xai", "moonshot": "moonshot",
	"qwen": "qwen", "cohere": "cohere", "mistral": "mistral", "azure": "azure",
}

// ModelsDevProviderKey resolves the models.dev provider id for this profile: an
// explicit modelsDevProvider wins, then the known preset mapping. "" means the
// profile does not identify a models.dev provider.
func (p ProviderProfile) ModelsDevProviderKey() string {
	if p.ModelsDevProvider != nil && *p.ModelsDevProvider != "" {
		return *p.ModelsDevProvider
	}
	if p.Preset != nil {
		if v, ok := presetModelsDevProviders[*p.Preset]; ok {
			return v
		}
	}
	return ""
}

// UnmarshalJSON adds the channel index to an error raised while decoding an
// upstream, so a bad channel field reports upstreams[i].<field> instead of losing
// the position. The happy path decodes once; only a failed decode re-walks the
// upstreams to find the index. The field rules themselves stay on Upstream.
func (p *ProviderProfile) UnmarshalJSON(data []byte) error {
	type providerProfileAlias ProviderProfile
	if err := json.Unmarshal(data, (*providerProfileAlias)(p)); err == nil {
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["upstreams"]; ok {
		var upstreams []json.RawMessage
		if err := json.Unmarshal(v, &upstreams); err == nil {
			for i, upstreamRaw := range upstreams {
				var upstream Upstream
				if err := json.Unmarshal(upstreamRaw, &upstream); err != nil {
					return fmt.Errorf("upstreams[%d]: %w", i, err)
				}
			}
		}
	}
	// The failure was not in an upstream; report the original decode error.
	return json.Unmarshal(data, (*providerProfileAlias)(p))
}

type CircuitBreakerSettings struct {
	Enabled          bool `json:"enabled"`
	FailureThreshold int  `json:"failureThreshold"`
	CooldownSeconds  int  `json:"cooldownSeconds"`
}
type Settings struct {
	WriteMode          string `json:"writeMode"`
	ConversationSource string `json:"conversationSource"`
	Proxy              struct {
		Host           string                 `json:"host"`
		Port           int                    `json:"port"`
		UserAgent      *string                `json:"userAgent,omitempty"`
		CircuitBreaker CircuitBreakerSettings `json:"circuitBreaker"`
		// RequestRetry is the number of additional credential retry rounds after
		// round 0 (nil = default 3, negative = default 3, 0 = no additional rounds).
		RequestRetry *int `json:"requestRetry,omitempty"`
		// MaxRetryCredentials caps distinct credentials tried per round (0 = all).
		MaxRetryCredentials int `json:"maxRetryCredentials,omitempty"`
		// MaxRetryInterval caps the cooldown wait between rounds in seconds
		// (nil = default 30, <=0 = never wait).
		MaxRetryInterval *int `json:"maxRetryInterval,omitempty"`
		// DisableCooling disables cooldown scheduling globally when true.
		DisableCooling *bool `json:"disableCooling,omitempty"`
		// TransientErrorCooldownSeconds cools 408/5xx-class and transport failures
		// (nil or 0 = legacy 60s, negative = disable).
		TransientErrorCooldownSeconds *int `json:"transientErrorCooldownSeconds,omitempty"`
		// RequestScopedErrors classifies upstream errors globally; a profile with
		// non-empty rules overrides these.
		RequestScopedErrors []RequestScopedError `json:"requestScopedErrors,omitempty"`
	} `json:"proxy"`
	Web struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"web"`
}

func (s *Settings) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		s.WriteMode = "gateway"
		s.ConversationSource = "sessionScan"
		s.Proxy.Host = "127.0.0.1"
		s.Proxy.Port = 43112
		s.Proxy.CircuitBreaker = CircuitBreakerSettings{Enabled: true, FailureThreshold: 3, CooldownSeconds: 60}
		s.Web.Host = "127.0.0.1"
		s.Web.Port = 43110
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if vv, ok := raw["writeMode"]; ok {
		if err := json.Unmarshal(vv, &s.WriteMode); err != nil {
			return fmt.Errorf("writeMode: %w", err)
		}
	}
	if vv, ok := raw["conversationSource"]; ok {
		var cs string
		if err := json.Unmarshal(vv, &cs); err != nil {
			return err
		}
		switch cs {
		case "proxy", "off", "sessionScan":
			s.ConversationSource = cs
		default:
			return fmt.Errorf("invalid conversationSource %q", cs)
		}
	} else if vv, ok := raw["injectOpenCodeAttribution"]; ok {
		var bval bool
		if err := json.Unmarshal(vv, &bval); err != nil {
			return fmt.Errorf("injectOpenCodeAttribution: %w", err)
		}
		if bval {
			s.ConversationSource = "proxy"
		} else {
			s.ConversationSource = "off"
		}
	}
	if vv, ok := raw["proxy"]; ok {
		if err := json.Unmarshal(vv, &s.Proxy); err != nil {
			return fmt.Errorf("proxy: %w", err)
		}
	}
	if vv, ok := raw["web"]; ok {
		if err := json.Unmarshal(vv, &s.Web); err != nil {
			return fmt.Errorf("web: %w", err)
		}
	}
	if s.WriteMode == "" {
		s.WriteMode = "gateway"
	}
	if s.ConversationSource == "" {
		s.ConversationSource = "sessionScan"
	}
	if s.Proxy.Host == "" {
		s.Proxy.Host = "127.0.0.1"
	}
	if s.Proxy.Port == 0 {
		s.Proxy.Port = 43112
	}
	if s.Web.Host == "" {
		s.Web.Host = "127.0.0.1"
	}
	if s.Web.Port == 0 {
		// circuitBreaker defaults (legacy JS compat): when the whole object is zero, backfill
		if s.Proxy.CircuitBreaker.FailureThreshold == 0 && s.Proxy.CircuitBreaker.CooldownSeconds == 0 {
			// Distinguish explicit "disabled: false with 0 thresholds" from missing object
			// by checking whether the raw JSON had a "circuitBreaker" key. The Unmarshal above
			// already filled s.Proxy from the raw "proxy" object, so a missing key leaves zeros.
			// We treat zeros as "missing" and backfill defaults; an explicit zero-threshold
			// config is not a valid intentional setting (min 1), so this is safe.
			if s.Proxy.CircuitBreaker.FailureThreshold == 0 {
				s.Proxy.CircuitBreaker.FailureThreshold = 3
			}
			if s.Proxy.CircuitBreaker.CooldownSeconds == 0 {
				s.Proxy.CircuitBreaker.CooldownSeconds = 60
			}
			// enabled defaults to true when the object was missing (zero value false would hide)
			// but we cannot tell missing vs explicit false. Preserve explicit false only when
			// thresholds were non-zero. Since we already decided thresholds were zero => missing,
			// set enabled to true.
			s.Proxy.CircuitBreaker.Enabled = true
		}
		s.Web.Port = 43110
	}
	return nil
}

type PiSwitchConfig struct {
	Version  uint32                     `json:"version"`
	Current  *string                    `json:"current"`
	Profiles map[string]ProviderProfile `json:"profiles"`
	Settings Settings                   `json:"settings"`
}

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

// normalizeConfigVersion is the single version floor shared by load and save, so
// the v1→v2 rule has one source instead of one copy per entry point.
func normalizeConfigVersion(v uint32) uint32 {
	if v < 2 {
		return 2
	}
	return v
}

func MigratedForSave(cfg PiSwitchConfig) PiSwitchConfig {
	out := cfg
	out.Version = normalizeConfigVersion(out.Version)
	if out.Settings.ConversationSource == "" {
		out.Settings.ConversationSource = "sessionScan"
	}
	switch out.Settings.ConversationSource {
	case "proxy", "off", "sessionScan":
	default:
		out.Settings.ConversationSource = "sessionScan"
	}
	return out
}

// ResolvePath returns the config file location used by every entry point
// (server, TUI, CLI). Precedence: PI_SWITCH_CONFIG > ~/.pi-switch/config.json,
// falling back to /tmp when the home directory cannot be determined.
func ResolvePath() string {
	if p := os.Getenv("PI_SWITCH_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "/tmp/pi-switch-config.json"
	}
	return filepath.Join(home, ".pi-switch", "config.json")
}

// writeFileAtomic writes data to path through a temp file in the same directory,
// fsyncs it and renames it into place. Writing to a temp file means path either
// keeps its previous content or holds the complete new content, and the rename
// preserves the temp file's 0600 mode.
func writeFileAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()
	// CreateTemp already uses 0600, but an unusual umask can only clear bits, so
	// set the mode explicitly to keep the guarantee.
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// SaveAtPath applies the save-time migration and writes cfg to path atomically,
// creating the parent directory as needed. Every entry point must go through
// this so that no caller skips the migration or the 0600 atomic write.
func SaveAtPath(cfg PiSwitchConfig, path string) error {
	cfg = MigratedForSave(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'))
}

// ChannelName returns the stable key of the i-th upstream ("" when unnamed).
func (p ProviderProfile) ChannelName(i int) string {
	if i < 0 || i >= len(p.Upstreams) {
		return ""
	}
	if p.Upstreams[i].Name == nil {
		return ""
	}
	return *p.Upstreams[i].Name
}

// ChannelView returns the models and exposed ids owned by the named channel.
func (p ProviderProfile) ChannelView(name string) ([]ModelEntry, []string) {
	for i := range p.Upstreams {
		if p.ChannelName(i) == name {
			return p.Upstreams[i].Models, p.Upstreams[i].ExposedModels
		}
	}
	return nil, nil
}

// ParseConfig parses config JSON with the same semantics every entry point uses:
// fields default from DefaultConfig, a present "profiles" replaces the
// placeholder profile (a config file without the key has no profiles), and any
// wrong field type is an error. Callers that write config back must parse through
// here first so they never persist raw JSON that the loader would reject.
func ParseConfig(b []byte) (PiSwitchConfig, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return PiSwitchConfig{}, err
	}
	cfg := DefaultConfig()
	if v, ok := raw["version"]; ok {
		if err := json.Unmarshal(v, &cfg.Version); err != nil {
			return PiSwitchConfig{}, fmt.Errorf("version: %w", err)
		}
	}
	if v, ok := raw["current"]; ok {
		if err := json.Unmarshal(v, &cfg.Current); err != nil {
			return PiSwitchConfig{}, fmt.Errorf("current: %w", err)
		}
	}
	// A config file that names its profiles (even an empty object) replaces the
	// DefaultConfig placeholder. A file without the key has no profiles at all —
	// the placeholder only belongs to the missing-file default, never to an
	// explicit config that happens to omit the key.
	cfg.Profiles = map[string]ProviderProfile{}
	if v, ok := raw["profiles"]; ok {
		// Decode profile by profile so the map key is part of the error path
		// (profiles.<name>.…), not just "profiles".
		var byName map[string]json.RawMessage
		if err := json.Unmarshal(v, &byName); err != nil {
			return PiSwitchConfig{}, fmt.Errorf("profiles: %w", err)
		}
		for name, profileRaw := range byName {
			var profile ProviderProfile
			if err := json.Unmarshal(profileRaw, &profile); err != nil {
				return PiSwitchConfig{}, fmt.Errorf("profiles.%s: %w", name, err)
			}
			cfg.Profiles[name] = profile
		}
	}
	if v, ok := raw["settings"]; ok {
		if err := json.Unmarshal(v, &cfg.Settings); err != nil {
			return PiSwitchConfig{}, fmt.Errorf("settings: %w", err)
		}
	}
	cfg.Version = normalizeConfigVersion(cfg.Version)
	return cfg, nil
}

// LoadConfigAtPath reads and parses the config file at path.
//
// A missing file is the only condition that yields DefaultConfig. Anything else
// — an unreadable file, malformed JSON, a field with the wrong type — is
// returned so a broken config can never masquerade as a default or empty one.
func LoadConfigAtPath(path string) (PiSwitchConfig, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultConfig(), "default (no file)", nil
		}
		return PiSwitchConfig{}, "", fmt.Errorf("read config %s: %w", path, err)
	}
	cfg, err := ParseConfig(b)
	if err != nil {
		return PiSwitchConfig{}, "", fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, path, nil
}

func (p ProviderProfile) PrimaryBaseURL() string {
	if len(p.Upstreams) > 0 && p.Upstreams[0].BaseURL != "" {
		return p.Upstreams[0].BaseURL
	}
	return p.BaseURL
}

func (p ProviderProfile) PrimaryAPIKey() string {
	if len(p.Upstreams) > 0 && p.Upstreams[0].APIKey != "" {
		return p.Upstreams[0].APIKey
	}
	return p.APIKey
}

func (p ProviderProfile) PrimaryHeaders() map[string]string {
	if len(p.Upstreams) > 0 && p.Upstreams[0].Headers != nil {
		return p.Upstreams[0].Headers
	}
	return p.Headers
}

func (p ProviderProfile) ResolvedUpstreams() []Upstream {
	if len(p.Upstreams) > 0 {
		return p.Upstreams
	}
	if p.BaseURL != "" || p.APIKey != "" || p.Headers != nil {
		return []Upstream{{BaseURL: p.BaseURL, APIKey: p.APIKey, Headers: p.Headers}}
	}
	return nil
}

func (m ModelEntry) EffectiveContextWindow() uint32 {
	if m.ContextWindow == 0 {
		return 128000
	}
	return m.ContextWindow
}

func (m ModelEntry) EffectiveMaxTokens() uint32 {
	if m.MaxTokens == 0 {
		return 16384
	}
	return m.MaxTokens
}
