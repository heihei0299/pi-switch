package gateway

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
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
	baseUrl := "http://" + host + ":" + itoa(port) + "/v1"
	providers := map[string]interface{}{}
	for name, prof := range cfg.Profiles {
		if prof.HasChannelPartitions() {
			for i := range prof.Upstreams {
				chName := prof.ChannelName(i)
				if chName == "" {
					continue
				}
				pool, exposed := prof.ChannelView(chName)
				if len(exposed) == 0 {
					continue
				}
				models := []interface{}{}
				for _, exposedID := range exposed {
					models = append(models, gatewayModelEntry(exposedID, pool, exposedID))
				}
				api := prof.Upstreams[i].API
				if api == "" {
					api = prof.API
				}
				if api == "" {
					api = cfg.Settings.GatewayAPI
				}
				providerKey := name + "/" + chName
				providers[providerKey] = map[string]interface{}{
					"api":     api,
					"baseUrl": baseUrl,
					"apiKey":  "pi-switch-proxy",
					"models":  models,
					"proxy":   false,
				}
			}
			continue
		}
		if len(prof.ExposedModels) == 0 {
			continue
		}
		// Legacy fallback: single provider per supplier (no channel suffix) for backward compat
		// Will be removed in 04 when top-level fields are deleted.
		pool := prof.Models
		exposed := prof.ExposedModels
		models := []interface{}{}
		for _, exposedID := range exposed {
			models = append(models, gatewayModelEntry(exposedID, pool, exposedID))
		}
		api := prof.API
		if api == "" {
			api = cfg.Settings.GatewayAPI
		}
		providerKey := name
		providers[providerKey] = map[string]interface{}{
			"api":     api,
			"baseUrl": baseUrl,
			"apiKey":  "pi-switch-proxy",
			"models":  models,
			"proxy":   false,
		}
	}
	return map[string]interface{}{"providers": providers}
}

// gatewayModelEntry builds one gateway model entry for a precomputed gateway id,
// looking the exposed id up in the given pool (channel pool or legacy top-level).
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

// isProvidersWrapper reports whether m is a providers wrapper (has "providers" key).
func isProvidersWrapper(m map[string]interface{}) bool {
	if m == nil {
		return false
	}
	_, ok := m["providers"]
	return ok
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

// DiffGateway mirrors webui/src/lib/gatewayDiff.ts diffGateway
// It handles both single gateway entry (old) and providers wrapper (new).
func DiffGateway(current, proposed map[string]interface{}) (added, removed, changed []string) {
	if isProvidersWrapper(current) || isProvidersWrapper(proposed) {
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
	if isProvidersWrapper(current) || isProvidersWrapper(proposed) {
		curProvs := getProviders(current)
		propProvs := getProviders(proposed)
		if curProvs == nil {
			curProvs = map[string]interface{}{}
		}
		if propProvs == nil {
			propProvs = map[string]interface{}{}
		}
		// If either is nil wrapper (current nil), pending is sum of prop providers/models?
		// For simplicity, use DiffGateway provider-level for added/removed/changed providers,
		// plus per-provider model-level diffs where provider exists in both.
		// Model-level counting: for each provider in both, count added/removed/changed models by bare id.
		pending := 0
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
				// provider added: count its models as pending
				if entry, ok := propProvs[k].(map[string]interface{}); ok {
					if models, ok := entry["models"].([]interface{}); ok {
						pending += len(models)
						if len(models) == 0 {
							pending++
						}
					} else {
						pending++
					}
				} else {
					pending++
				}
			}
		}
		for k := range curSet {
			if !propSet[k] {
				if entry, ok := curProvs[k].(map[string]interface{}); ok {
					if models, ok := entry["models"].([]interface{}); ok {
						pending += len(models)
						if len(models) == 0 {
							pending++
						}
					} else {
						pending++
					}
				} else {
					pending++
				}
			}
		}
		for k := range propSet {
			if !curSet[k] {
				continue
			}
			curEntry, _ := curProvs[k].(map[string]interface{})
			propEntry, _ := propProvs[k].(map[string]interface{})
			if curEntry == nil || propEntry == nil {
				continue
			}
			// provider fields diff (api/baseUrl/proxy) counts as 1 if differs outside models
			curCopy := map[string]interface{}{}
			propCopy := map[string]interface{}{}
			for kk, vv := range curEntry {
				if kk != "models" {
					curCopy[kk] = vv
				}
			}
			for kk, vv := range propEntry {
				if kk != "models" {
					propCopy[kk] = vv
				}
			}
			ca, _ := json.Marshal(curCopy)
			ba, _ := json.Marshal(propCopy)
			if string(ca) != string(ba) {
				pending++
			}
			// model-level diff per provider
			curModels, _ := curEntry["models"].([]interface{})
			propModels, _ := propEntry["models"].([]interface{})
			curByID := map[string]map[string]interface{}{}
			propByID := map[string]map[string]interface{}{}
			for _, m := range curModels {
				if mm, ok := m.(map[string]interface{}); ok {
					if id, _ := mm["id"].(string); id != "" {
						curByID[id] = mm
					}
				}
			}
			for _, m := range propModels {
				if mm, ok := m.(map[string]interface{}); ok {
					if id, _ := mm["id"].(string); id != "" {
						propByID[id] = mm
					}
				}
			}
			for id := range propByID {
				if _, ok := curByID[id]; !ok {
					pending++
				} else {
					a, _ := json.Marshal(curByID[id])
					b, _ := json.Marshal(propByID[id])
					if string(a) != string(b) {
						pending++
					}
				}
			}
			for id := range curByID {
				if _, ok := propByID[id]; !ok {
					pending++
				}
			}
		}
		return pending
	}
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

var wrapperGeneratedKeys = map[string]bool{"providers": true}

// MergeGatewayExtra merges non-generated top-level keys and per-model extra fields from current into proposed.
// It preserves current's extra top-level keys and merges headers/compat/extra per model id.
// Supports both single provider entry (old) and providers wrapper (new).
func MergeGatewayExtra(current, proposed map[string]interface{}) map[string]interface{} {
	if current == nil {
		return proposed
	}
	if isProvidersWrapper(current) || isProvidersWrapper(proposed) {
		merged := map[string]interface{}{}
		for k, v := range proposed {
			merged[k] = v
		}
		// top-level non-generated for wrapper (only providers is generated)
		for k, v := range current {
			if !wrapperGeneratedKeys[k] {
				if _, exists := merged[k]; !exists {
					merged[k] = v
				} else {
					merged[k] = v
				}
			}
		}
		curProvs, _ := current["providers"].(map[string]interface{})
		propProvs, _ := merged["providers"].(map[string]interface{})
		if curProvs == nil {
			curProvs = map[string]interface{}{}
		}
		if propProvs == nil {
			propProvs = map[string]interface{}{}
		}
		// per-provider merge
		for key, propVal := range propProvs {
			propMap, ok := propVal.(map[string]interface{})
			if !ok {
				continue
			}
			curVal, hasCur := curProvs[key]
			if !hasCur {
				continue
			}
			curMap, ok := curVal.(map[string]interface{})
			if !ok {
				continue
			}
			// top-level non-generated per provider entry
			for ck, cv := range curMap {
				if !generatedKeys[ck] {
					if _, exists := propMap[ck]; !exists {
						propMap[ck] = cv
					}
				}
			}
			// per-model extra within this provider
			curModels, _ := curMap["models"].([]interface{})
			propModels, _ := propMap["models"].([]interface{})
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
								if curMap, ok := curVal.(map[string]interface{}); ok {
									if propMapInner, ok := mm[extraKey].(map[string]interface{}); ok {
										for ek, ev := range curMap {
											if _, exists := propMapInner[ek]; !exists {
												propMapInner[ek] = ev
											}
										}
										mm[extraKey] = propMapInner
										propModels[i] = mm
									}
								}
							}
						}
					}
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
			propMap["models"] = propModels
			propProvs[key] = propMap
		}
		merged["providers"] = propProvs
		// Ensure no spurious top-level models/api etc for wrapper
		delete(merged, "models")
		delete(merged, "api")
		delete(merged, "baseUrl")
		delete(merged, "apiKey")
		delete(merged, "proxy")
		return merged
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
				merged[k] = v
			}
		}
	}
	// per-model extra: headers/compat/extra
	curModels, _ := current["models"].([]interface{})
	propModels, _ := merged["models"].([]interface{})
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
	var curByID, propByID map[string]map[string]interface{}
	if isProvidersWrapper(current) || isProvidersWrapper(proposed) {
		buildByID := func(m map[string]interface{}) map[string]map[string]interface{} {
			out := map[string]map[string]interface{}{}
			provs := getProviders(m)
			for providerKey, pv := range provs {
				entry, _ := pv.(map[string]interface{})
				if entry == nil {
					continue
				}
				models, _ := entry["models"].([]interface{})
				for _, mod := range models {
					if mm, ok := mod.(map[string]interface{}); ok {
						if id, _ := mm["id"].(string); id != "" {
							gid := id
							if providerKey == cfg.Settings.ProviderPrefix || providerKey == "pi-switch" {
								gid = id
							} else if !strings.Contains(id, "/") {
								gid = providerKey + "/" + id
							} else {
								gid = id
							}
							out[gid] = mm
						}
					}
				}
			}
			return out
		}
		curByID = buildByID(current)
		propByID = buildByID(proposed)
	} else {
		curByID = map[string]map[string]interface{}{}
		if ms, _ := current["models"].([]interface{}); ms != nil {
			for _, m := range ms {
				if mm, ok := m.(map[string]interface{}); ok {
					if id, _ := mm["id"].(string); id != "" {
						curByID[id] = mm
					}
				}
			}
		}
		propByID = map[string]map[string]interface{}{}
		if ms, _ := proposed["models"].([]interface{}); ms != nil {
			for _, m := range ms {
				if mm, ok := m.(map[string]interface{}); ok {
					if id, _ := mm["id"].(string); id != "" {
						propByID[id] = mm
					}
				}
			}
		}
	}
	statusOf := func(id string) string {
		cur, ok := curByID[id]
		if !ok {
			return "pending"
		}
		prop, ok := propByID[id]
		if !ok {
			return "pending"
		}
		// Normalize id to bare for comparison: current may have prefixed id, proposed has bare
		normalize := func(m map[string]interface{}) map[string]interface{} {
			out := map[string]interface{}{}
			for k, v := range m {
				out[k] = v
			}
			if idVal, ok := out["id"].(string); ok {
				if idx := strings.LastIndex(idVal, "/"); idx >= 0 {
					out["id"] = idVal[idx+1:]
				}
			}
			return out
		}
		a, _ := json.Marshal(normalize(cur))
		b, _ := json.Marshal(normalize(prop))
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
		if prof.HasChannelPartitions() {
			for i := range prof.Upstreams {
				ch := prof.ChannelName(i)
				_, exposed := prof.ChannelView(ch)
				items := []PreviewGroupItem{}
				for _, eid := range exposed {
					gid := name + "/" + ch + "/" + eid
					items = append(items, PreviewGroupItem{ID: eid, Status: statusOf(gid)})
				}
				groups = append(groups, PreviewGroup{Supplier: name, Channel: ch, Models: items})
			}
			continue
		}
		items := []PreviewGroupItem{}
		for _, eid := range prof.ExposedModels {
			gid := name + "/" + eid
			items = append(items, PreviewGroupItem{ID: eid, Status: statusOf(gid)})
		}
		groups = append(groups, PreviewGroup{Supplier: name, Channel: "", Models: items})
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
