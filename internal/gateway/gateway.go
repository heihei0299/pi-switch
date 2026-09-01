package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

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
					if len(m.Input) > 0 {
						entry["input"] = m.Input
					}
					if m.Reasoning != nil && *m.Reasoning {
						entry["compat"] = map[string]interface{}{"supportsDeveloperRole": false}
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

// DiffGateway mirrors webui/src/lib/gatewayDiff.ts diffGateway
func DiffGateway(current, proposed map[string]interface{}) (added, removed, changed []string) {
	if current == nil {
		for k := range proposed {
			added = append(added, k)
		}
		return
	}
	curSet := map[string]bool{}
	propSet := map[string]bool{}
	for k := range current {
		curSet[k] = true
	}
	for k := range proposed {
		propSet[k] = true
	}
	for k := range propSet {
		if !curSet[k] {
			added = append(added, k)
		} else {
			a, _ := json.Marshal(current[k])
			b, _ := json.Marshal(proposed[k])
			if string(a) != string(b) {
				changed = append(changed, k)
			}
		}
	}
	for k := range curSet {
		if !propSet[k] {
			removed = append(removed, k)
		}
	}
	return
}

func ComputePendingCount(current, proposed map[string]interface{}) int {
	a, r, c := DiffGateway(current, proposed)
	return len(a) + len(r) + len(c)
}

var generatedKeys = map[string]bool{
	"api":     true,
	"baseUrl": true,
	"apiKey":  true,
	"models":  true,
	"proxy":   true,
}

// MergeGatewayExtra merges non-generated top-level keys and per-model extra fields from current into proposed.
// It preserves current's extraTop-level keys and merges headers/compat/extra per model id.
func MergeGatewayExtra(current, proposed map[string]interface{}) map[string]interface{} {
	if current == nil {
		return proposed
	}
	merged := map[string]interface{}{}
	for k, v := range proposed {
		merged[k] = v
	}
	// top-level non-generated
	for k, v := range current {
		if !generatedKeys[k] {
			if _, exists := merged[k]; !exists {
				merged[k] = v
			} else {
				// keep current's value if proposed doesn't already have? Actually spec says merge, we keep current's extra if proposed missing. If both have, keep proposed? But test expects keep current's extraTop. So if not in generated, and current has, we preserve it.
				// If merged already has from proposed (shouldn't happen for non-generated), we keep current's? Let's prefer current for non-generated.
				merged[k] = v
			}
		}
	}
	// per-model extra: headers/compat/extra
	curModels, _ := current["models"].([]interface{})
	propModels, _ := merged["models"].([]interface{})
	// index current models by id
	curByID := map[string]map[string]interface{}{}
	for _, m := range curModels {
		if mm, ok := m.(map[string]interface{}); ok {
			if id, _ := mm["id"].(string); id != "" {
				curByID[id] = mm
			}
		}
	}
	for i, m := range propModels {
		mm, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := mm["id"].(string)
		if cur, exists := curByID[id]; exists {
			for _, extraKey := range []string{"headers", "compat", "extra"} {
				if curVal, has := cur[extraKey]; has {
					if _, exists := mm[extraKey]; !exists {
						mm[extraKey] = curVal
						propModels[i] = mm
					} else {
						// merge inner map if both are maps
						if curMap, ok := curVal.(map[string]interface{}); ok {
							if propMap, ok := mm[extraKey].(map[string]interface{}); ok {
								for ek, ev := range curMap {
									if _, exists := propMap[ek]; !exists {
										propMap[ek] = ev
									}
								}
								mm[extraKey] = propMap
								propModels[i] = mm
							}
						}
					}
				}
			}
			// also preserve any non-standard fields from current that are not in generated model keys (id,contextWindow,maxTokens,cost,input,reasoning,name)
			// For simplicity, copy any key not in standard set if not present
			standard := map[string]bool{"id": true, "contextWindow": true, "maxTokens": true, "cost": true, "input": true, "reasoning": true, "name": true, "headers": true, "compat": true, "extra": true}
			for ck, cv := range cur {
				if !standard[ck] {
					if _, exists := mm[ck]; !exists {
						mm[ck] = cv
						propModels[i] = mm
					}
				}
			}
		}
	}
	merged["models"] = propModels
	return merged
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
	// if edited is whole providers wrapper? handleGatewayPublish may pass providers wrapper, but here we assume edited is single entry
	// merge extra
	var currentEntry map[string]interface{}
	if cur, ok := providers[gatewayID]; ok {
		if curMap, ok := cur.(map[string]interface{}); ok {
			currentEntry = curMap
		}
	}
	merged := MergeGatewayExtra(currentEntry, edited)
	providers[gatewayID] = merged
	b, _ := json.MarshalIndent(m, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0644); err != nil {
		return err
	}
	// backup
	backupPath := path + ".backup"
	_ = os.WriteFile(backupPath, b, 0644)
	// handle atomic rename with pid suffix? Use tmp then rename
	// To avoid race, we already have tmp, just rename
	// Also try pid suffix if needed for test? The tmp already.
	return os.Rename(tmp, path)
}

// Helper for syncing settings after publish
func SyncSettingsFromGateway(cfg *config.PiSwitchConfig, gateway map[string]interface{}) bool {
	changed := false
	if api, ok := gateway["api"].(string); ok && api != "" && api != cfg.Settings.GatewayAPI {
		cfg.Settings.GatewayAPI = api
		changed = true
	}
	if bu, ok := gateway["baseUrl"].(string); ok && bu != "" {
		// parse host/port
		// expect http://host:port/v1
		trimmed := strings.TrimPrefix(bu, "http://")
		trimmed = strings.TrimPrefix(trimmed, "https://")
		// remove path after /
		if idx := strings.Index(trimmed, "/"); idx >= 0 {
			trimmed = trimmed[:idx]
		}
		// split host:port
		host := trimmed
		port := 0
		if idx := strings.LastIndex(trimmed, ":"); idx >= 0 {
			host = trimmed[:idx]
			pStr := trimmed[idx+1:]
			for _, c := range pStr {
				if c < '0' || c > '9' {
					host = trimmed
					port = 0
					break
				}
			}
			if port == 0 {
				// parse
				var p int
				for _, ch := range pStr {
					p = p*10 + int(ch-'0')
				}
				port = p
			}
		}
		if host == "0.0.0.0" || host == "::" || host == "[::]" {
			host = "127.0.0.1"
		}
		if host != "" && host != cfg.Settings.Proxy.Host {
			cfg.Settings.Proxy.Host = host
			changed = true
		}
		if port != 0 && port != cfg.Settings.Proxy.Port {
			cfg.Settings.Proxy.Port = port
			changed = true
		}
	}
	return changed
}
