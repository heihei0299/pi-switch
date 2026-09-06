package catalog

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// internal/catalog: models.dev 模型元数据快照（拉取 + 本地缓存 + 查找 + 补缺）。
// 只依赖标准库。网关预览/发布用它补齐提议条目的缺失模型元数据；池内数据不写回。

const (
	// FetchURLVar overrides the snapshot source in tests.
	FetchURLVar = "PI_SWITCH_CATALOG_URL"
	// CachePathVar overrides the cache file path (tests).
	CachePathVar = "PI_SWITCH_CATALOG"
	// DefaultFetchURL is the models.dev snapshot endpoint.
	DefaultFetchURL = "https://models.dev/api.json"
	// TTL matches the documented 24h cache.
	TTL = 24 * time.Hour
	// FetchTimeout bounds a single refresh attempt.
	FetchTimeout = 15 * time.Second
)

func fetchURL() string {
	if u := os.Getenv(FetchURLVar); u != "" {
		return u
	}
	return DefaultFetchURL
}

// CachePath returns the snapshot cache file path.
func CachePath() string {
	if p := os.Getenv(CachePathVar); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/pi-switch-models-dev.json"
	}
	return filepath.Join(home, ".pi-switch", "cache", "models-dev.json")
}

// Meta is one models.dev model entry, normalized to pi-switch vocabulary.
type Meta struct {
	Name          string
	Reasoning     bool
	Input         []string
	ContextWindow uint64
	MaxTokens     uint64
	CostInput     float64
	CostOutput    float64
	CacheRead     float64
}

type apiModel struct {
	Name       string `json:"name"`
	Reasoning  bool   `json:"reasoning"`
	Modalities struct {
		Input []string `json:"input"`
	} `json:"modalities"`
	Limit struct {
		Context uint64 `json:"context"`
		Output  uint64 `json:"output"`
	} `json:"limit"`
	Cost struct {
		Input     float64 `json:"input"`
		Output    float64 `json:"output"`
		CacheRead float64 `json:"cache_read"`
	} `json:"cost"`
}

// Snapshot is a parsed models.dev payload indexed by bare model id.
// A bare id maps to at most one Meta: ambiguous (multi-lab same tail) and
// missing ids simply miss — never mismatch.
type Snapshot struct {
	byBare         map[string]Meta
	byProviderBare map[string]Meta
}

func bareID(id string) string {
	if idx := strings.LastIndex(id, "/"); idx >= 0 {
		return id[idx+1:]
	}
	return id
}

// ParseSnapshot indexes a models.dev api.json payload.
func ParseSnapshot(data []byte) (Snapshot, error) {
	var providers map[string]struct {
		Models map[string]apiModel `json:"models"`
	}
	if err := json.Unmarshal(data, &providers); err != nil {
		return Snapshot{}, err
	}
	hits := map[string][]Meta{}
	snap := Snapshot{byBare: map[string]Meta{}, byProviderBare: map[string]Meta{}}
	for provName, prov := range providers {
		for fullID, m := range prov.Models {
			bare := bareID(fullID)
			meta := Meta{
				Name:          m.Name,
				Reasoning:     m.Reasoning,
				Input:         m.Modalities.Input,
				ContextWindow: m.Limit.Context,
				MaxTokens:     m.Limit.Output,
				CostInput:     m.Cost.Input,
				CostOutput:    m.Cost.Output,
				CacheRead:     m.Cost.CacheRead,
			}
			hits[bare] = append(hits[bare], meta)
			snap.byProviderBare[provName+"/"+bare] = meta
		}
	}
	for bare, list := range hits {
		if len(list) == 1 {
			snap.byBare[bare] = list[0]
		}
	}
	return snap, nil
}

// Lookup finds the unique catalog entry for a (possibly prefixed) model id.
func (s Snapshot) Lookup(id string) (Meta, bool) {
	if s.byBare == nil {
		return Meta{}, false
	}
	m, ok := s.byBare[bareID(id)]
	return m, ok
}

// LookupWithProvider finds the catalog entry for id, preferring provider/bare when provider is given.
// It tries provider/bare first, then falls back to unique bare.
func (s Snapshot) LookupWithProvider(id string, provider string) (Meta, bool) {
	if provider != "" && s.byProviderBare != nil {
		if m, ok := s.byProviderBare[provider+"/"+bareID(id)]; ok {
			return m, true
		}
	}
	return s.Lookup(id)
}

// Empty reports whether the snapshot holds no entries.
func (s Snapshot) Empty() bool { return len(s.byBare) == 0 && len(s.byProviderBare) == 0 }

// asNumber reads any JSON/Go numeric value as float64.
func asNumber(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f, true
		}
		return 0, false
	}
	return 0, false
}

// fillFloat sets m[key] when the catalog value is non-zero and the entry
// value is missing or zero. Numbers normalize to float64 (JSON domain).
func fillFloat(m map[string]interface{}, key string, val float64) bool {
	if val == 0 {
		return false
	}
	if f, ok := asNumber(m[key]); ok && f != 0 {
		return false
	}
	m[key] = val
	return true
}

// FillMissing 只补缺失字段，已有值不覆盖，返回是否有目录数据补入。
// 数值一律写 float64（JSON 域归一，避免与落盘 float64 序列化分叉）。
//   - name 为空补；contextWindow/maxTokens 缺失或 0 且目录非 0 则补
//   - input 缺失或空补；reasoning 缺键则按目录值补（含 false）
//   - cost 缺失整设（含显式 cacheWrite:0，保 pending 收敛）；已存在则补 0 值子字段
//   - 存量 cost 缺 cacheWrite 键时补零归一（保 pending 收敛），该归一不计入返回值
func FillMissing(entry map[string]interface{}, meta Meta) bool {
	filled := false
	if s, _ := entry["name"].(string); strings.TrimSpace(s) == "" && strings.TrimSpace(meta.Name) != "" {
		entry["name"] = meta.Name
		filled = true
	}
	if fillFloat(entry, "contextWindow", float64(meta.ContextWindow)) {
		filled = true
	}
	if fillFloat(entry, "maxTokens", float64(meta.MaxTokens)) {
		filled = true
	}
	if fillInput(entry, meta.Input) {
		filled = true
	}
	if _, has := entry["reasoning"]; !has {
		entry["reasoning"] = meta.Reasoning
		filled = true
	}
	if raw, ok := entry["cost"]; !ok || raw == nil {
		if meta.CostInput != 0 || meta.CostOutput != 0 || meta.CacheRead != 0 {
			entry["cost"] = map[string]interface{}{
				"input":      meta.CostInput,
				"output":     meta.CostOutput,
				"cacheRead":  meta.CacheRead,
				"cacheWrite": float64(0),
			}
			filled = true
		}
	} else if cm, ok := raw.(map[string]interface{}); ok {
		if fillFloat(cm, "input", meta.CostInput) {
			filled = true
		}
		if fillFloat(cm, "output", meta.CostOutput) {
			filled = true
		}
		if fillFloat(cm, "cacheRead", meta.CacheRead) {
			filled = true
		}
		if _, has := cm["cacheWrite"]; !has {
			cm["cacheWrite"] = float64(0)
		}
	}
	return filled
}

// fillInput fills entry input when missing or empty. Numbers normalize to
// []interface{} (JSON domain).
func fillInput(entry map[string]interface{}, input []string) bool {
	empty := false
	if entry["input"] == nil {
		empty = true
	} else if arr, ok := entry["input"].([]interface{}); ok && len(arr) == 0 {
		empty = true
	} else if arr, ok := entry["input"].([]string); ok && len(arr) == 0 {
		empty = true
	}
	if !empty || len(input) == 0 {
		return false
	}
	in := make([]interface{}, 0, len(input))
	for _, v := range input {
		in = append(in, v)
	}
	entry["input"] = in
	return true
}

func overwriteFloat(m map[string]interface{}, key string, val float64) bool {
	if val == 0 {
		return false
	}
	if f, ok := asNumber(m[key]); ok && f == val {
		return false
	}
	m[key] = val
	return true
}

func overwriteInput(entry map[string]interface{}, input []string) bool {
	if len(input) == 0 {
		return false
	}
	// compare existing input
	existing := entry["input"]
	if existing == nil {
		in := make([]interface{}, 0, len(input))
		for _, v := range input {
			in = append(in, v)
		}
		entry["input"] = in
		return true
	}
	var cur []string
	switch v := existing.(type) {
	case []interface{}:
		cur = make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				cur = append(cur, s)
			}
		}
	case []string:
		cur = v
	default:
		// unknown type, overwrite
		in := make([]interface{}, 0, len(input))
		for _, e := range input {
			in = append(in, e)
		}
		entry["input"] = in
		return true
	}
	if len(cur) == len(input) {
		match := true
		for i := range cur {
			if cur[i] != input[i] {
				match = false
				break
			}
		}
		if match {
			return false
		}
	}
	in := make([]interface{}, 0, len(input))
	for _, v := range input {
		in = append(in, v)
	}
	entry["input"] = in
	return true
}

// FillOverwrite overwrites entry fields with catalog meta when catalog has non-zero values.
// It returns true if any field changed. cacheWrite normalization is not counted.
func FillOverwrite(entry map[string]interface{}, meta Meta) bool {
	changed := false
	if strings.TrimSpace(meta.Name) != "" {
		if s, _ := entry["name"].(string); strings.TrimSpace(s) != strings.TrimSpace(meta.Name) {
			entry["name"] = meta.Name
			changed = true
		}
	}
	if overwriteFloat(entry, "contextWindow", float64(meta.ContextWindow)) {
		changed = true
	}
	if overwriteFloat(entry, "maxTokens", float64(meta.MaxTokens)) {
		changed = true
	}
	if overwriteInput(entry, meta.Input) {
		changed = true
	}
	if _, has := entry["reasoning"]; !has {
		entry["reasoning"] = meta.Reasoning
		changed = true
	} else if entry["reasoning"] != meta.Reasoning {
		entry["reasoning"] = meta.Reasoning
		changed = true
	}
	if raw, ok := entry["cost"]; !ok || raw == nil {
		if meta.CostInput != 0 || meta.CostOutput != 0 || meta.CacheRead != 0 {
			entry["cost"] = map[string]interface{}{
				"input":      meta.CostInput,
				"output":     meta.CostOutput,
				"cacheRead":  meta.CacheRead,
				"cacheWrite": float64(0),
			}
			changed = true
		}
	} else if cm, ok := raw.(map[string]interface{}); ok {
		if overwriteFloat(cm, "input", meta.CostInput) {
			changed = true
		}
		if overwriteFloat(cm, "output", meta.CostOutput) {
			changed = true
		}
		if overwriteFloat(cm, "cacheRead", meta.CacheRead) {
			changed = true
		}
		if _, has := cm["cacheWrite"]; !has {
			cm["cacheWrite"] = float64(0)
		}
	}
	return changed
}

func providerHint(id string) string {
	if idx := strings.Index(id, "/"); idx > 0 {
		return id[:idx]
	}
	return ""
}

// FillModels fills a gateway models array in place, returning per-entry counts.
// Non-object entries and empty ids count as skipped.
// It prefers provider/bare lookup when id contains a supplier prefix (e.g. supplier/model or supplier/channel/model)
// and overwrites existing values when catalog provides non-zero values (allows correction of stale defaults like 128000→1048576).
func FillModels(models []interface{}, snap Snapshot) (enriched, skipped int) {
	for _, m := range models {
		mm, ok := m.(map[string]interface{})
		if !ok {
			skipped++
			continue
		}
		id, _ := mm["id"].(string)
		if id == "" {
			skipped++
			continue
		}
		var meta Meta
		var found bool
		hint := providerHint(id)
		if hint != "" {
			meta, found = snap.LookupWithProvider(id, hint)
		} else {
			meta, found = snap.Lookup(id)
		}
		if !found {
			skipped++
			continue
		}
		if FillOverwrite(mm, meta) {
			enriched++
		} else {
			skipped++
		}
	}
	return enriched, skipped
}

// EnrichSummary mirrors the preview enrich payload.
type EnrichSummary struct {
	Enriched int    `json:"enriched"`
	Skipped  int    `json:"skipped"`
	Stale    bool   `json:"stale"`
	Warning  string `json:"warning"`
}

func writeCache(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Ensure loads the snapshot, refreshing a missing/stale cache. It never
// fails hard: refresh failure falls back to stale cache (stale=true + warning),
// no cache at all yields an empty snapshot with a warning.
func Ensure() (snap Snapshot, stale bool, warning string) {
	path := CachePath()
	client := &http.Client{Timeout: FetchTimeout}
	if fi, err := os.Stat(path); err == nil && time.Since(fi.ModTime()) < TTL {
		if data, err := os.ReadFile(path); err == nil {
			if s, err := ParseSnapshot(data); err == nil {
				return s, false, ""
			}
		}
	}
	// fetch fresh (also when cache corrupt/missing)
	var err error
	if resp, ferr := client.Get(fetchURL()); ferr == nil {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err = statusErr(resp.StatusCode); err == nil {
			var s Snapshot
			if s, err = ParseSnapshot(data); err == nil {
				_ = writeCache(path, data)
				return s, false, ""
			}
		}
	} else {
		err = ferr
	}
	// fall back to stale cache
	if data, rerr := os.ReadFile(path); rerr == nil {
		if s, perr := ParseSnapshot(data); perr == nil && !s.Empty() {
			return s, true, "models.dev refresh failed, using stale cache"
		}
	}
	if err == nil {
		err = errNoCache
	}
	return Snapshot{}, false, "models.dev unavailable: " + err.Error()
}

func statusErr(code int) error {
	if code >= 200 && code < 300 {
		return nil
	}
	return errString("models.dev: unexpected status " + itoa(code))
}

// itoa without fmt (this package stays stdlib-light).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

var errNoCache = errString("no cache and refresh failed")

type errString string

func (e errString) Error() string { return string(e) }
