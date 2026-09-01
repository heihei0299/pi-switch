package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/heihei0299/pi-switch/internal/config"
)

func ModelsPath() string {
	if p := os.Getenv("PI_SWITCH_MODELS"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/pi-switch-models.json"
	}
	return filepath.Join(home, ".pi", "agent", "models.json")
}

func loadModelsValue(path string) (map[string]interface{}, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{"providers": map[string]interface{}{}}, nil
	}
	var v map[string]interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return map[string]interface{}{"providers": map[string]interface{}{}}, nil
	}
	if _, ok := v["providers"]; !ok {
		v["providers"] = map[string]interface{}{}
	}
	return v, nil
}

func BuildProposedGatewayEntry(cfg config.PiSwitchConfig) map[string]interface{} {
	host := cfg.Settings.Proxy.Host
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	port := cfg.Settings.Proxy.Port
	models := []interface{}{}
	for name, prof := range cfg.Profiles {
		if len(prof.ExposedModels) == 0 && len(prof.Models) == 0 {
			continue
		}
		exposed := prof.ExposedModels
		if len(exposed) == 0 {
			for _, m := range prof.Models {
				exposed = append(exposed, m.ID)
			}
		}
		for _, exposedID := range exposed {
			var entry map[string]interface{}
			found := false
			for _, m := range prof.Models {
				if m.ID == exposedID {
					entry = map[string]interface{}{
						"id":            name + "/" + exposedID,
						"contextWindow": m.ContextWindow,
						"maxTokens":     m.MaxTokens,
					}
					if m.Name != nil {
						entry["name"] = *m.Name
					}
					if m.Cost != nil {
						entry["cost"] = m.Cost
					}
					found = true
					break
				}
			}
			if !found {
				entry = map[string]interface{}{
					"id":            name + "/" + exposedID,
					"contextWindow": uint32(128000),
					"maxTokens":     uint32(16384),
				}
			}
			models = append(models, entry)
		}
	}
	return map[string]interface{}{
		"api":     cfg.Settings.GatewayAPI,
		"baseUrl": "http://" + host + ":" + itoa(port) + "/v1",
		"apiKey":  "pi-switch-proxy",
		"models":  models,
		"proxy":   false,
	}
}

func itoa(n int) string { return jsonNumber(n) }
func jsonNumber(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func Publish(cfg config.PiSwitchConfig, edited map[string]interface{}) error {
	path := ModelsPath()
	dir := filepath.Dir(path)
	_ = os.MkdirAll(dir, 0755)
	m, err := loadModelsValue(path)
	if err != nil {
		return err
	}
	providers, ok := m["providers"].(map[string]interface{})
	if !ok {
		providers = map[string]interface{}{}
		m["providers"] = providers
	}
	gatewayID := cfg.Settings.ProviderPrefix
	providers[gatewayID] = edited
	b, _ := json.MarshalIndent(m, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
