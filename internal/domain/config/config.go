package config

import (
	"encoding/json"
	"fmt"

	"github.com/heihei0299/pi-switch/internal/domain/provider"
)

type ModelCost = provider.ModelCost
type ModelEntry = provider.ModelEntry
type RequestScopedError = provider.RequestScopedError
type Upstream = provider.Upstream
type ProviderProfile = provider.Profile

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

// normalizeConfigVersion is the single version floor shared by load and save.
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

// ParseConfig parses config JSON with the same semantics every entry point uses:
// fields default from DefaultConfig, a present "profiles" replaces the
// placeholder profile (a config file without the key has no profiles), and any
// wrong field type is an error. Callers that write config back must parse through
// here first so they never persist raw JSON that the loader would reject.
func ParseConfig(b []byte, defaults PiSwitchConfig) (PiSwitchConfig, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return PiSwitchConfig{}, err
	}
	cfg := defaults
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
