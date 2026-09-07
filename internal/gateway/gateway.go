package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

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

func BuildProposedGatewayEntry(cfg config.PiSwitchConfig) map[string]interface{} {
	host := cfg.Settings.Proxy.Host
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	port := cfg.Settings.Proxy.Port
	baseUrl := "http://" + host + ":" + itoa(port) + "/v1"
	providers := map[string]interface{}{}
	for name, prof := range cfg.Profiles {
		for i := range prof.Upstreams {
			channel := prof.Upstreams[i]
			channelName := prof.ChannelName(i)
			if channelName == "" || channel.API == "" || len(channel.ExposedModels) == 0 {
				continue
			}
			models := []interface{}{}
			for _, exposedID := range channel.ExposedModels {
				models = append(models, gatewayModelEntry(exposedID, channel.Models, exposedID))
			}
			providers[name+"/"+channelName] = map[string]interface{}{
				"api":     channel.API,
				"baseUrl": baseUrl,
				"apiKey":  "pi-switch-proxy",
				"models":  models,
				"proxy":   false,
			}
		}
	}
	return map[string]interface{}{"providers": providers}
}

// gatewayModelEntry builds one gateway model entry by looking the exposed id up
// in its channel's model pool.
func gatewayModelEntry(id string, pool []config.ModelEntry, exposedID string) map[string]interface{} {
	for _, m := range pool {
		if m.ID != exposedID {
			continue
		}
		entry := map[string]interface{}{
			"id":            id,
			"contextWindow": m.ContextWindow,
			"maxTokens":     m.MaxTokens,
		}
		if m.Name != nil {
			entry["name"] = *m.Name
		}
		if m.Cost != nil {
			// cost 显式带 cacheWrite（含 0）：落盘形态经 webui draft round-trip
			// 必含 cacheWrite:0；若提议侧用 *ModelCost（omitempty 省 0），
			// JSON 序列化恒差，preview pending 恒=1，同步横幅永不消失。
			entry["cost"] = map[string]interface{}{
				"input":      m.Cost.Input,
				"output":     m.Cost.Output,
				"cacheRead":  m.Cost.CacheRead,
				"cacheWrite": m.Cost.CacheWrite,
			}
		}
		if len(m.Input) > 0 {
			entry["input"] = m.Input
		}
		if m.Reasoning != nil && *m.Reasoning {
			entry["compat"] = map[string]interface{}{"supportsDeveloperRole": false}
		}
		return entry
	}
	return map[string]interface{}{
		"id":            id,
		"contextWindow": uint32(128000),
		"maxTokens":     uint32(16384),
	}
}
func itoa(n int) string { return jsonNumber(n) }
func jsonNumber(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func getProviders(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	if provs, ok := m["providers"].(map[string]interface{}); ok {
		return provs
	}
	return nil
}

// DiffGateway mirrors webui/src/lib/gatewayDiff.ts diffGateway for providers.
func DiffGateway(current, proposed map[string]interface{}) (added, removed, changed []string) {
	curProvs := getProviders(current)
	propProvs := getProviders(proposed)
	if curProvs == nil {
		curProvs = map[string]interface{}{}
	}
	if propProvs == nil {
		propProvs = map[string]interface{}{}
	}
	if current == nil {
		for k := range propProvs {
			added = append(added, k)
		}
		return
	}
	// provider-level added/removed
	curSet := map[string]bool{}
	propSet := map[string]bool{}
	for k := range curProvs {
		curSet[k] = true
	}
	for k := range propProvs {
		propSet[k] = true
	}
	for k := range propSet {
		if !curSet[k] {
			added = append(added, k)
		}
	}
	for k := range curSet {
		if !propSet[k] {
			removed = append(removed, k)
		}
	}
	// per-provider model-level + provider fields diff -> mark provider as changed
	// For pending per providerKey+modelId, we count model-level diffs as changed entries with key = providerKey + "/" + modelId
	// However to keep DiffGateway simple, we report providerKey as changed if its models or api differ.
	// Model-level diffs are accounted in ComputePendingCount via per-model counts.
	for k := range propSet {
		if !curSet[k] {
			continue
		}
		curEntry, _ := curProvs[k].(map[string]interface{})
		propEntry, _ := propProvs[k].(map[string]interface{})
		if curEntry == nil || propEntry == nil {
			continue
		}
		a, _ := json.Marshal(curEntry)
		b, _ := json.Marshal(propEntry)
		if string(a) != string(b) {
			changed = append(changed, k)
		}
	}
	return
}

func ComputePendingCount(current, proposed map[string]interface{}) int {
	curProvs := getProviders(current)
	propProvs := getProviders(proposed)
	if curProvs == nil {
		curProvs = map[string]interface{}{}
	}
	if propProvs == nil {
		propProvs = map[string]interface{}{}
	}
	pending := 0
	for key, value := range propProvs {
		if _, ok := curProvs[key]; ok {
			continue
		}
		if entry, ok := value.(map[string]interface{}); ok {
			if models, ok := entry["models"].([]interface{}); ok && len(models) > 0 {
				pending += len(models)
			} else {
				pending++
			}
		} else {
			pending++
		}
	}
	for key, value := range curProvs {
		if _, ok := propProvs[key]; ok {
			continue
		}
		if entry, ok := value.(map[string]interface{}); ok {
			if models, ok := entry["models"].([]interface{}); ok && len(models) > 0 {
				pending += len(models)
			} else {
				pending++
			}
		} else {
			pending++
		}
	}
	for key, value := range propProvs {
		curValue, ok := curProvs[key]
		if !ok {
			continue
		}
		curEntry, curOK := curValue.(map[string]interface{})
		propEntry, propOK := value.(map[string]interface{})
		if !curOK || !propOK {
			pending++
			continue
		}
		curCopy := map[string]interface{}{}
		propCopy := map[string]interface{}{}
		for field, fieldValue := range curEntry {
			if field != "models" {
				curCopy[field] = fieldValue
			}
		}
		for field, fieldValue := range propEntry {
			if field != "models" {
				propCopy[field] = fieldValue
			}
		}
		curJSON, _ := json.Marshal(curCopy)
		propJSON, _ := json.Marshal(propCopy)
		if string(curJSON) != string(propJSON) {
			pending++
		}
		curModels, _ := curEntry["models"].([]interface{})
		propModels, _ := propEntry["models"].([]interface{})
		curByID := map[string]map[string]interface{}{}
		propByID := map[string]map[string]interface{}{}
		for _, model := range curModels {
			if entry, ok := model.(map[string]interface{}); ok {
				if id, _ := entry["id"].(string); id != "" {
					curByID[id] = entry
				}
			}
		}
		for _, model := range propModels {
			if entry, ok := model.(map[string]interface{}); ok {
				if id, _ := entry["id"].(string); id != "" {
					propByID[id] = entry
				}
			}
		}
		for id, propModel := range propByID {
			curModel, exists := curByID[id]
			if !exists {
				pending++
				continue
			}
			curJSON, _ := json.Marshal(curModel)
			propJSON, _ := json.Marshal(propModel)
			if string(curJSON) != string(propJSON) {
				pending++
			}
		}
		for id := range curByID {
			if _, exists := propByID[id]; !exists {
				pending++
			}
		}
	}
	return pending
}

var generatedKeys = map[string]bool{
	"api":     true,
	"baseUrl": true,
	"apiKey":  true,
	"models":  true,
	"proxy":   true,
}

// MergeGatewayExtra merges non-generated provider and model fields from current
// into the proposed providers wrapper.
func MergeGatewayExtra(current, proposed map[string]interface{}) map[string]interface{} {
	if current == nil {
		return proposed
	}
	merged := map[string]interface{}{"providers": map[string]interface{}{}}
	for key, value := range proposed {
		if key != "providers" {
			merged[key] = value
		}
	}
	curProvs := getProviders(current)
	propProvs := getProviders(proposed)
	if propProvs == nil {
		propProvs = map[string]interface{}{}
	}
	mergedProvs := merged["providers"].(map[string]interface{})
	for key, value := range propProvs {
		propMap, ok := value.(map[string]interface{})
		if !ok {
			mergedProvs[key] = value
			continue
		}
		curMap, _ := curProvs[key].(map[string]interface{})
		if curMap == nil {
			mergedProvs[key] = propMap
			continue
		}
		for field, fieldValue := range curMap {
			if !generatedKeys[field] {
				if _, exists := propMap[field]; !exists {
					propMap[field] = fieldValue
				}
			}
		}
		curModels, _ := curMap["models"].([]interface{})
		propModels, _ := propMap["models"].([]interface{})
		curByID := map[string]map[string]interface{}{}
		for _, model := range curModels {
			if entry, ok := model.(map[string]interface{}); ok {
				if id, _ := entry["id"].(string); id != "" {
					curByID[id] = entry
				}
			}
		}
		for i, model := range propModels {
			entry, ok := model.(map[string]interface{})
			if !ok {
				continue
			}
			id, _ := entry["id"].(string)
			if old, exists := curByID[id]; exists {
				for _, field := range []string{"headers", "compat", "extra"} {
					oldValue, hasOld := old[field]
					if !hasOld {
						continue
					}
					newValue, hasNew := entry[field]
					if !hasNew {
						entry[field] = oldValue
						continue
					}
					oldMap, oldOK := oldValue.(map[string]interface{})
					newMap, newOK := newValue.(map[string]interface{})
					if oldOK && newOK {
						for key, value := range oldMap {
							if _, exists := newMap[key]; !exists {
								newMap[key] = value
							}
						}
					}
				}
				standard := map[string]bool{"id": true, "contextWindow": true, "maxTokens": true, "cost": true, "input": true, "reasoning": true, "name": true, "headers": true, "compat": true, "extra": true}
				for field, fieldValue := range old {
					if !standard[field] {
						if _, exists := entry[field]; !exists {
							entry[field] = fieldValue
						}
					}
				}
			}
			propModels[i] = entry
		}
		propMap["models"] = propModels
		mergedProvs[key] = propMap
	}
	return merged
}

func Publish(cfg config.PiSwitchConfig, edited map[string]interface{}) error {
	path := ModelsPath()
	dir := filepath.Dir(path)
	_ = os.MkdirAll(dir, 0755)
	provs, ok := edited["providers"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("gateway.providers is required")
	}
	b, _ := json.MarshalIndent(map[string]interface{}{"providers": provs}, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// PreviewGroup is one supplier/channel group of exposed candidates for the
// gateway preview: each model carries published/pending status against current.
type PreviewGroup struct {
	Supplier string             `json:"supplier"`
	Channel  string             `json:"channel"`
	Models   []PreviewGroupItem `json:"models"`
}

// PreviewGroupItem is one exposed candidate with its publish status.
type PreviewGroupItem struct {
	ID     string `json:"id"`
	Status string `json:"status"` // published | pending
}

// BuildPreviewGroups groups proposed exposed candidates by supplier/channel
// (deterministic: suppliers sorted, channels/upstreams and exposed in order).
// status is published when current holds a JSON-equal entry, else pending.
// Current-only ids return as sorted removed (stale entries awaiting cleanup).
func BuildPreviewGroups(cfg config.PiSwitchConfig, current, proposed map[string]interface{}) ([]PreviewGroup, []string) {
	buildByID := func(m map[string]interface{}) map[string]map[string]interface{} {
		out := map[string]map[string]interface{}{}
		for providerKey, value := range getProviders(m) {
			entry, _ := value.(map[string]interface{})
			if entry == nil {
				continue
			}
			models, _ := entry["models"].([]interface{})
			for _, model := range models {
				if mm, ok := model.(map[string]interface{}); ok {
					if id, _ := mm["id"].(string); id != "" {
						out[providerKey+"/"+id] = mm
					}
				}
			}
		}
		return out
	}
	curByID := buildByID(current)
	propByID := buildByID(proposed)
	statusOf := func(id string) string {
		cur, ok := curByID[id]
		if !ok {
			return "pending"
		}
		prop, ok := propByID[id]
		if !ok {
			return "pending"
		}
		a, _ := json.Marshal(cur)
		b, _ := json.Marshal(prop)
		if string(a) == string(b) {
			return "published"
		}
		return "pending"
	}
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	groups := []PreviewGroup{}
	for _, name := range names {
		prof := cfg.Profiles[name]
		for i := range prof.Upstreams {
			channel := prof.Upstreams[i]
			ch := prof.ChannelName(i)
			if ch == "" {
				continue
			}
			items := []PreviewGroupItem{}
			for _, eid := range channel.ExposedModels {
				gid := name + "/" + ch + "/" + eid
				items = append(items, PreviewGroupItem{ID: eid, Status: statusOf(gid)})
			}
			groups = append(groups, PreviewGroup{Supplier: name, Channel: ch, Models: items})
		}
	}
	var removed []string
	for id := range curByID {
		if _, ok := propByID[id]; !ok {
			removed = append(removed, id)
		}
	}
	sort.Strings(removed)
	if removed == nil {
		removed = []string{}
	}
	return groups, removed
}
