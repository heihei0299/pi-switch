package config

import (
	"encoding/json"
	"fmt"
	"os"
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

func (u Upstream) EffectiveResponsesMode(fallback string) string {
	if u.ResponsesMode != "" {
		return u.ResponsesMode
	}
	return fallback
}

func ValidateUpstreamAPI(u Upstream, profile ProviderProfile) error {
	if u.API == "" {
		return fmt.Errorf("api is required for each upstream")
	}
	effectiveMode := u.EffectiveResponsesMode(profile.ResponsesMode)
	allowed := map[string]bool{"openai-completions": true, "openai-responses": true, "anthropic-messages": true, "google-generative-ai": true}
	if !allowed[u.API] {
		return fmt.Errorf("unsupported api %s", u.API)
	}
	if effectiveMode == "" {
		effectiveMode = "auto"
	}
	if effectiveMode == "auto" {
		return nil
	}
	if effectiveMode == "passthrough" && u.API != "openai-responses" {
		return fmt.Errorf("responsesMode passthrough requires api openai-responses, got %s", u.API)
	}
	if effectiveMode == "convert" && u.API != "openai-completions" {
		return fmt.Errorf("responsesMode convert requires api openai-completions, got %s", u.API)
	}
	if effectiveMode != "passthrough" && effectiveMode != "convert" {
		return fmt.Errorf("invalid responsesMode %q", effectiveMode)
	}
	return nil
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
		_ = json.Unmarshal(vv, &s.WriteMode)
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
		if err := json.Unmarshal(vv, &bval); err == nil {
			if bval {
				s.ConversationSource = "proxy"
			} else {
				s.ConversationSource = "off"
			}
		}
	}
	if vv, ok := raw["proxy"]; ok {
		_ = json.Unmarshal(vv, &s.Proxy)
	}
	if vv, ok := raw["web"]; ok {
		_ = json.Unmarshal(vv, &s.Web)
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
				API:           "openai-completions",
				ResponsesMode: "auto",
				BaseURL:       "https://api.openai.com/v1",
				APIKey:        "sk-prototype-not-real",
				Upstreams: []Upstream{{
					Name:          &channel,
					API:           "openai-completions",
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
	out := cfg
	if out.Version < 2 {
		out.Version = 2
	}
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

func LoadConfigAtPath(path string) (PiSwitchConfig, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return DefaultConfig(), "default (no file)", nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return DefaultConfig(), "default (bad json)", nil
	}
	cfg := DefaultConfig()
	if v, ok := raw["version"]; ok {
		_ = json.Unmarshal(v, &cfg.Version)
	}
	if v, ok := raw["current"]; ok {
		_ = json.Unmarshal(v, &cfg.Current)
	}
	if v, ok := raw["profiles"]; ok {
		cfg.Profiles = map[string]ProviderProfile{}
		_ = json.Unmarshal(v, &cfg.Profiles)
		if cfg.Profiles == nil {
			cfg.Profiles = map[string]ProviderProfile{}
		}
	}
	if v, ok := raw["settings"]; ok {
		_ = json.Unmarshal(v, &cfg.Settings)
	}
	if cfg.Settings.WriteMode == "" {
		cfg.Settings.WriteMode = "gateway"
	}
	if cfg.Settings.ConversationSource == "" {
		cfg.Settings.ConversationSource = "sessionScan"
	}
	if cfg.Settings.Proxy.Host == "" {
		cfg.Settings.Proxy.Host = "127.0.0.1"
	}
	if cfg.Settings.Proxy.Port == 0 {
		cfg.Settings.Proxy.Port = 43112
	}
	if cfg.Settings.Web.Host == "" {
		cfg.Settings.Web.Host = "127.0.0.1"
	}
	if cfg.Settings.Web.Port == 0 {
		cfg.Settings.Web.Port = 43110
	}
	if cfg.Settings.Proxy.CircuitBreaker.FailureThreshold == 0 && cfg.Settings.Proxy.CircuitBreaker.CooldownSeconds == 0 {
		if cfg.Settings.Proxy.CircuitBreaker.FailureThreshold == 0 {
			cfg.Settings.Proxy.CircuitBreaker.FailureThreshold = 3
		}
		if cfg.Settings.Proxy.CircuitBreaker.CooldownSeconds == 0 {
			cfg.Settings.Proxy.CircuitBreaker.CooldownSeconds = 60
		}
		cfg.Settings.Proxy.CircuitBreaker.Enabled = true
	}
	if cfg.Version < 2 {
		cfg.Version = 2
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
