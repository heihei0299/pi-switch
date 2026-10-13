package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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
	// RequestRetry overrides the profile/global retry budget; nil/negative inherit and zero allows no extra rounds.
	RequestRetry *int `json:"requestRetry,omitempty"`
	// DisableCooling overrides global cooling when non-nil.
	DisableCooling *bool        `json:"disableCooling,omitempty"`
	Models         []ModelEntry `json:"models,omitempty"`
	ExposedModels  []string     `json:"exposedModels,omitempty"`
}

// UnmarshalJSON keeps missing, null, and empty exposure distinct at the input edge.
func (u *Upstream) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["exposedModels"]; ok && bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
		return fmt.Errorf("exposedModels must not be null")
	}
	type upstreamAlias Upstream
	return json.Unmarshal(data, (*upstreamAlias)(u))
}

func (u Upstream) MarshalJSON() ([]byte, error) {
	type upstreamAlias Upstream
	var exposed *[]string
	if u.ExposedModels != nil {
		exposed = &u.ExposedModels
	}
	return json.Marshal(struct {
		upstreamAlias
		ExposedModels *[]string `json:"exposedModels,omitempty"`
	}{upstreamAlias: upstreamAlias(u), ExposedModels: exposed})
}

func (u Upstream) EffectiveResponsesMode(fallback string) string {
	if u.ResponsesMode != "" {
		return u.ResponsesMode
	}
	return fallback
}

// EffectiveAPI resolves the api the channel actually speaks.
func (u Upstream) EffectiveAPI(fallback string) string {
	if u.API != "" {
		return u.API
	}
	return fallback
}

type Profile struct {
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
	// RequestRetry overrides the global retry budget; nil/negative inherit and zero allows no extra rounds.
	RequestRetry *int `json:"requestRetry,omitempty"`
	// DisableCooling overrides global cooling when non-nil.
	DisableCooling *bool `json:"disableCooling,omitempty"`
	// RequestScopedErrors classifies upstream errors for this profile.
	RequestScopedErrors []RequestScopedError `json:"requestScopedErrors,omitempty"`
}

var presetModelsDevProviders = map[string]string{
	"openai": "openai", "anthropic": "anthropic", "google": "google",
	"deepseek": "deepseek", "xai": "xai", "moonshot": "moonshot",
	"qwen": "qwen", "cohere": "cohere", "mistral": "mistral", "azure": "azure",
}

// ModelsDevProviderKey prefers an explicit source over the built-in preset mapping.
func (p Profile) ModelsDevProviderKey() string {
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

// UnmarshalJSON adds the channel index to upstream decode errors.
func (p *Profile) UnmarshalJSON(data []byte) error {
	type profileAlias Profile
	if err := json.Unmarshal(data, (*profileAlias)(p)); err == nil {
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
	return json.Unmarshal(data, (*profileAlias)(p))
}

// ChannelName returns the stable name of an upstream, if one is declared.
func (p Profile) ChannelName(i int) string {
	if i < 0 || i >= len(p.Upstreams) || p.Upstreams[i].Name == nil {
		return ""
	}
	return *p.Upstreams[i].Name
}

// ChannelView returns models owned by the named upstream.
func (p Profile) ChannelView(name string) ([]ModelEntry, []string) {
	for i := range p.Upstreams {
		if p.ChannelName(i) == name {
			return p.Upstreams[i].Models, p.Upstreams[i].ExposedModels
		}
	}
	return nil, nil
}

func (p Profile) PrimaryBaseURL() string {
	if len(p.Upstreams) > 0 && p.Upstreams[0].BaseURL != "" {
		return p.Upstreams[0].BaseURL
	}
	return p.BaseURL
}

func (p Profile) PrimaryAPIKey() string {
	if len(p.Upstreams) > 0 && p.Upstreams[0].APIKey != "" {
		return p.Upstreams[0].APIKey
	}
	return p.APIKey
}

func (p Profile) PrimaryHeaders() map[string]string {
	if len(p.Upstreams) > 0 {
		return MergeHeaders(p.Headers, p.Upstreams[0].Headers)
	}
	return MergeHeaders(p.Headers, nil)
}

// ResolvedUpstreams preserves legacy flat profiles as one runtime upstream.
func (p Profile) ResolvedUpstreams() []Upstream {
	if len(p.Upstreams) > 0 {
		return p.Upstreams
	}
	if p.BaseURL != "" || p.APIKey != "" || p.Headers != nil {
		return []Upstream{{BaseURL: p.BaseURL, APIKey: p.APIKey, Headers: p.Headers}}
	}
	return nil
}

// MergeHeaders applies channel overrides without treating header case as distinct.
func MergeHeaders(profile, channel map[string]string) map[string]string {
	merged := make(map[string]string, len(profile)+len(channel))
	for _, source := range []map[string]string{profile, channel} {
		for key, value := range source {
			if key != "" {
				merged[http.CanonicalHeaderKey(key)] = value
			}
		}
	}
	return merged
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
