package config

import (
	"encoding/json"
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
}

type Upstream struct {
	BaseURL string            `json:"baseUrl"`
	APIKey  string            `json:"apiKey"`
	Headers map[string]string `json:"headers,omitempty"`
	Weight  *uint32           `json:"weight,omitempty"`
	Name    *string           `json:"name,omitempty"`
}

type ProviderProfile struct {
	API           string                 `json:"api"`
	ResponsesMode string                 `json:"responsesMode"`
	BaseURL       string                 `json:"baseUrl"`
	APIKey        string                 `json:"apiKey"`
	Upstreams     []Upstream             `json:"upstreams,omitempty"`
	Models        []ModelEntry           `json:"models"`
	Headers       map[string]string      `json:"headers,omitempty"`
	ExposedModels []string               `json:"exposedModels,omitempty"`
	ModelMap      map[string]interface{} `json:"modelMap,omitempty"`
	UserAgent     *string                `json:"userAgent,omitempty"`
	Preset             *string                `json:"preset,omitempty"`
	ModelsDevProvider *string                `json:"modelsDevProvider,omitempty"`
	Proxy         *bool                  `json:"proxy,omitempty"`
}

type Settings struct {
	ProviderPrefix     string `json:"providerPrefix"`
	WriteMode          string `json:"writeMode"`
	GatewayAPI         string `json:"gatewayApi"`
	ConversationSource string `json:"conversationSource"`
	Proxy              struct {
		Host     string   `json:"host"`
		Port     int      `json:"port"`
		Failover []string `json:"failover,omitempty"`
	} `json:"proxy"`
	Web struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"web"`
}

type PiSwitchConfig struct {
	Version  uint32                     `json:"version"`
	Current  *string                    `json:"current"`
	Profiles map[string]ProviderProfile `json:"profiles"`
	Settings Settings                   `json:"settings"`
}

func DefaultConfig() PiSwitchConfig {
	pp := "test-provider"
	return PiSwitchConfig{
		Version: 2,
		Current: &pp,
		Profiles: map[string]ProviderProfile{
			"test-provider": {
				API:           "openai-completions",
				ResponsesMode: "auto",
				BaseURL:       "https://api.openai.com/v1",
				APIKey:        "sk-prototype-not-real",
				Models: []ModelEntry{
					{ID: "gpt-4o-mini", ContextWindow: 128000, MaxTokens: 16384, Cost: &ModelCost{Input: 0.15, Output: 0.6, CacheRead: 0.075}},
				},
			},
		},
		Settings: Settings{
			ProviderPrefix:     "pi-switch",
			WriteMode:          "gateway",
			GatewayAPI:         "openai-completions",
			ConversationSource: "sessionScan",
			Proxy: struct {
				Host     string   `json:"host"`
				Port     int      `json:"port"`
				Failover []string `json:"failover,omitempty"`
			}{Host: "127.0.0.1", Port: 43112},
			Web: struct {
				Host string `json:"host"`
				Port int    `json:"port"`
			}{Host: "127.0.0.1", Port: 43110},
		},
	}
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
		var sRaw map[string]json.RawMessage
		if err := json.Unmarshal(v, &sRaw); err == nil {
			if vv, ok := sRaw["providerPrefix"]; ok {
				_ = json.Unmarshal(vv, &cfg.Settings.ProviderPrefix)
			}
			if vv, ok := sRaw["writeMode"]; ok {
				_ = json.Unmarshal(vv, &cfg.Settings.WriteMode)
			}
			if vv, ok := sRaw["gatewayApi"]; ok {
				_ = json.Unmarshal(vv, &cfg.Settings.GatewayAPI)
			}
			if vv, ok := sRaw["conversationSource"]; ok {
				_ = json.Unmarshal(vv, &cfg.Settings.ConversationSource)
			} else if vv, ok := sRaw["injectOpenCodeAttribution"]; ok {
				var bval bool
				if err := json.Unmarshal(vv, &bval); err == nil {
					if bval {
						cfg.Settings.ConversationSource = "proxy"
					} else {
						cfg.Settings.ConversationSource = "off"
					}
				}
			}
			if vv, ok := sRaw["proxy"]; ok {
				_ = json.Unmarshal(vv, &cfg.Settings.Proxy)
			}
			if vv, ok := sRaw["web"]; ok {
				_ = json.Unmarshal(vv, &cfg.Settings.Web)
			}
		}
	}
	if cfg.Settings.ProviderPrefix == "" {
		cfg.Settings.ProviderPrefix = "pi-switch"
	}
	if cfg.Settings.WriteMode == "" {
		cfg.Settings.WriteMode = "gateway"
	}
	if cfg.Settings.GatewayAPI == "" {
		cfg.Settings.GatewayAPI = "openai-completions"
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
