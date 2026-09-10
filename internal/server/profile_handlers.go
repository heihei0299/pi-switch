package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/catalog"
	"github.com/heihei0299/pi-switch/internal/config"
)

func handleGetConfig(c *gin.Context) {
	cfg, src, _ := config.LoadConfigAtPath(configPath())
	c.JSON(200, gin.H{"source": src, "config": cfg})
}

func handlePutConfig(c *gin.Context) {
	raw, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	path := configPath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		c.JSON(400, gin.H{"error": "invalid json"})
		return
	}
	// validate responsesMode
	if m, ok := v.(map[string]interface{}); ok {
		if profiles, ok := m["profiles"].(map[string]interface{}); ok {
			for name, pv := range profiles {
				if pm, ok := pv.(map[string]interface{}); ok {
					if api, _ := pm["api"].(string); api != "" {
						if mode, _ := pm["responsesMode"].(string); mode != "" {
							if err := validateProfileResponsesMode(api, mode); err != nil {
								c.JSON(400, gin.H{"error": fmt.Sprintf("profile %s: %s", name, err.Error())})
								return
							}
						}
					}
				}
			}
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	// A failed rename means the config was NOT replaced. Reporting success here
	// would tell the operator their edit was saved while the old file is still in
	// place (a realistic outcome on Windows when the target is locked).
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		c.JSON(500, gin.H{"error": "failed to save config: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func handleGetState(c *gin.Context) {
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	c.JSON(200, gin.H{"current": cfg.Current, "profiles": cfg.Profiles, "settings": cfg.Settings})
}

func handleListProfiles(c *gin.Context) {
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	c.JSON(200, cfg.Profiles)
}

func handleGetProfile(c *gin.Context) {
	name := c.Param("name")
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	prof, ok := cfg.Profiles[name]
	if !ok {
		c.JSON(404, gin.H{"error": fmt.Sprintf("unknown profile '%s'", name)})
		return
	}
	c.JSON(200, gin.H{"name": name, "profile": prof, "providerId": name})
}

func validateProfileResponsesMode(api, mode string) error {
	if mode == "" {
		mode = "auto"
	}
	if mode == "auto" {
		return nil
	}
	if mode == "passthrough" && api != "openai-responses" {
		return fmt.Errorf("responsesMode passthrough requires api openai-responses, got %s", api)
	}
	if mode == "convert" && api != "openai-completions" {
		return fmt.Errorf("responsesMode convert requires api openai-completions, got %s", api)
	}
	if mode != "passthrough" && mode != "convert" {
		return fmt.Errorf("invalid responsesMode %q", mode)
	}
	return nil
}

// isPersistError distinguishes a failed write from a rejected input, so the
// handler can keep answering 400 for validation and 500 for storage.
func isPersistError(err error) bool {
	if err == nil {
		return false
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return true
	}
	// Rename failures come back as *os.LinkError, not *os.PathError.
	var linkErr *os.LinkError
	return errors.As(err, &linkErr)
}

// CreateProfile is the single implementation behind POST /api/profiles and
// `pi-switch provider add`: it validates the profile (responsesMode
// compatibility, shape, retry knobs) and refuses to overwrite an existing
// supplier. Callers map its errors to their own surface (400/500 for HTTP,
// exit code + stderr for the CLI).
func CreateProfile(name string, prof config.ProviderProfile) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name required")
	}
	if err := validateResponsesMode(prof); err != nil {
		return err
	}
	if err := validateProviderProfile(prof); err != nil {
		return err
	}
	if err := validateRetryFields(prof); err != nil {
		return err
	}
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]config.ProviderProfile{}
	}
	if _, exists := cfg.Profiles[name]; exists {
		return errors.New("profile already exists")
	}
	// 不在这里默认暴露全部：新建供应商的模型默认不暴露，需显式 expose，
	// 与"空 exposed = 不暴露"一致。
	cfg.Profiles[name] = prof
	return saveConfig(cfg)
}

func handlePostProfile(c *gin.Context) {
	var body struct {
		Name    string          `json:"name"`
		Profile json.RawMessage `json:"profile"`
	}
	raw, _ := c.GetRawData()

	// quick validation for responsesMode in raw json
	{
		var rawMap map[string]interface{}
		if err := json.Unmarshal(raw, &rawMap); err == nil {
			// check profile wrapper
			if prof, ok := rawMap["profile"].(map[string]interface{}); ok {
				if api, _ := prof["api"].(string); api != "" {
					if mode, _ := prof["responsesMode"].(string); mode != "" {
						if err := validateProfileResponsesMode(api, mode); err != nil {
							c.JSON(400, gin.H{"error": err.Error()})
							return
						}
					}
				}
			}
			// direct api field (for PUT /api/config profiles)
			if profiles, ok := rawMap["profiles"].(map[string]interface{}); ok {
				for _, pv := range profiles {
					if pm, ok := pv.(map[string]interface{}); ok {
						if api, _ := pm["api"].(string); api != "" {
							if mode, _ := pm["responsesMode"].(string); mode != "" {
								if err := validateProfileResponsesMode(api, mode); err != nil {
									c.JSON(400, gin.H{"error": err.Error()})
									return
								}
							}
						}
					}
				}
			}
			// single profile case
			if api, ok := rawMap["api"].(string); ok {
				if mode, _ := rawMap["responsesMode"].(string); mode != "" {
					if err := validateProfileResponsesMode(api, mode); err != nil {
						c.JSON(400, gin.H{"error": err.Error()})
						return
					}
				}
			}
		}
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		c.JSON(400, gin.H{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		c.JSON(400, gin.H{"error": "name required"})
		return
	}
	var prof config.ProviderProfile
	if err := json.Unmarshal(body.Profile, &prof); err != nil {
		c.JSON(400, gin.H{"error": fmt.Sprintf("invalid profile: %v", err)})
		return
	}
	// validate responsesMode compatibility
	// 校验、重名与落盘都在 CreateProfile 里（与 CLI 共用一份实现），
	// handler 只负责把它映射成 HTTP 错误码：校验类 400、落盘失败 500。
	if err := CreateProfile(body.Name, prof); err != nil {
		if isPersistError(err) {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "backup": nil})
}

func handlePutProfile(c *gin.Context) {
	name := c.Param("name")
	var body struct {
		Profile    json.RawMessage `json:"profile"`
		RenameFrom *string         `json:"renameFrom"`
	}
	raw, _ := c.GetRawData()

	// quick validation for responsesMode in raw json
	{
		var rawMap map[string]interface{}
		if err := json.Unmarshal(raw, &rawMap); err == nil {
			// check profile wrapper
			if prof, ok := rawMap["profile"].(map[string]interface{}); ok {
				if api, _ := prof["api"].(string); api != "" {
					if mode, _ := prof["responsesMode"].(string); mode != "" {
						if err := validateProfileResponsesMode(api, mode); err != nil {
							c.JSON(400, gin.H{"error": err.Error()})
							return
						}
					}
				}
			}
			// direct api field (for PUT /api/config profiles)
			if profiles, ok := rawMap["profiles"].(map[string]interface{}); ok {
				for _, pv := range profiles {
					if pm, ok := pv.(map[string]interface{}); ok {
						if api, _ := pm["api"].(string); api != "" {
							if mode, _ := pm["responsesMode"].(string); mode != "" {
								if err := validateProfileResponsesMode(api, mode); err != nil {
									c.JSON(400, gin.H{"error": err.Error()})
									return
								}
							}
						}
					}
				}
			}
			// single profile case
			if api, ok := rawMap["api"].(string); ok {
				if mode, _ := rawMap["responsesMode"].(string); mode != "" {
					if err := validateProfileResponsesMode(api, mode); err != nil {
						c.JSON(400, gin.H{"error": err.Error()})
						return
					}
				}
			}
		}
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		c.JSON(400, gin.H{"error": "invalid json"})
		return
	}
	var prof config.ProviderProfile
	if err := json.Unmarshal(body.Profile, &prof); err != nil {
		c.JSON(400, gin.H{"error": fmt.Sprintf("invalid profile: %v", err)})
		return
	}
	if err := validateResponsesMode(prof); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := validateProviderProfile(prof); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := validateRetryFields(prof); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]config.ProviderProfile{}
	}
	// handle rename
	if body.RenameFrom != nil && *body.RenameFrom != "" && *body.RenameFrom != name {
		delete(cfg.Profiles, *body.RenameFrom)
	}
	cfg.Profiles[name] = prof
	// if current points to renamed old, update?
	if err := saveConfig(cfg); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "backup": nil})
}

func handleDeleteProfile(c *gin.Context) {
	name := c.Param("name")
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	if _, ok := cfg.Profiles[name]; !ok {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	delete(cfg.Profiles, name)
	// if current == name, clear
	if cfg.Current != nil && *cfg.Current == name {
		cfg.Current = nil
	}
	if err := saveConfig(cfg); err != nil {
		c.JSON(500, gin.H{"error": "failed to save config: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "backup": nil})
}

// DuplicateProfile copies a profile under a new name. It is the single
// implementation behind POST /api/profiles/:name/duplicate and
// `pi-switch provider duplicate`, and it never overwrites an existing supplier.
func DuplicateProfile(src, as string) error {
	as = strings.TrimSpace(as)
	if as == "" {
		return errors.New("--as <new> required")
	}
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	prof, ok := cfg.Profiles[src]
	if !ok {
		return fmt.Errorf("profile %q not found", src)
	}
	if _, exists := cfg.Profiles[as]; exists {
		return fmt.Errorf("target %q already exists", as)
	}
	cfg.Profiles[as] = prof
	return saveConfig(cfg)
}

func handleDuplicateProfile(c *gin.Context) {
	name := c.Param("name")
	var body struct {
		As string `json:"as"`
	}
	raw, _ := c.GetRawData()
	_ = json.Unmarshal(raw, &body)
	if err := DuplicateProfile(name, body.As); err != nil {
		switch {
		case strings.Contains(err.Error(), "not found"):
			c.JSON(404, gin.H{"error": "not found"})
		case strings.Contains(err.Error(), "already exists"):
			c.JSON(400, gin.H{"error": "target exists"})
		case isPersistError(err):
			c.JSON(500, gin.H{"error": "failed to save config: " + err.Error()})
		default:
			c.JSON(400, gin.H{"error": err.Error()})
		}
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// FetchUpstreamModelIDs lists the models a profile's primary upstream reports.
// It is the read-only half of POST /api/profiles/:name/fetch-models (the handler
// additionally enriches and persists for channel-directed fetches) and the whole
// of `pi-switch provider fetch-models`. Returns a human-readable reason instead
// of an error so callers can surface it as their own kind of failure.
func FetchUpstreamModelIDs(prof config.ProviderProfile) ([]string, string) {
	return fetchUpstreamIDs(prof.PrimaryBaseURL(), prof.PrimaryAPIKey(), nil)
}

// TestProfileUpstream performs the read-only upstream probe behind
// POST /api/profiles/:name/test and `pi-switch provider test`: it GETs the
// smallest models endpoint, never writes, and never touches request stats.
// success=false with a reason is a finding, not an error, so callers decide how
// to surface it (HTTP 200 with success:false, CLI exit code).
func TestProfileUpstream(prof config.ProviderProfile) (success bool, message string, responseMs int64) {
	baseURL := strings.TrimRight(prof.PrimaryBaseURL(), "/")
	apiKey := prof.PrimaryAPIKey()
	if baseURL == "" {
		return false, "baseUrl is empty", 0
	}
	start := time.Now()
	client := &http.Client{Timeout: 5 * time.Second}
	urls := []string{baseURL + "/models", baseURL + "/v1/models"}
	var lastErr string
	for _, u := range urls {
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		ms := time.Since(start).Milliseconds()
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return false, fmt.Sprintf("upstream HTTP %d: invalid api key or no permission", resp.StatusCode), ms
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Sprintf("upstream HTTP %d: %s", resp.StatusCode, truncateForTest(body))
			continue
		}
		return true, "ok", ms
	}
	return false, "unreachable: " + lastErr, time.Since(start).Milliseconds()
}

func handleTestProfile(c *gin.Context) {
	name := c.Param("name")
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	prof, ok := cfg.Profiles[name]
	if !ok {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	success, message, ms := TestProfileUpstream(prof)
	c.JSON(200, gin.H{"success": success, "message": message, "responseTimeMs": ms})
}

func truncateForTest(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	if s == "" {
		s = "(empty body)"
	}
	return s
}

// handleFetchModelsForChannel fetches /v1/models with the named channel's
// credentials, merges new ids (enriched) into that channel's pool only and
// persists. Existing entries are never modified; other channels untouched.
func handleFetchModelsForChannel(c *gin.Context, cfg config.PiSwitchConfig, name string, prof config.ProviderProfile, channel string) {
	idx := ensureMutationChannel(&prof, channel)
	if idx < 0 {
		c.JSON(400, gin.H{"error": fmt.Sprintf("unknown channel %q", channel)})
		return
	}
	u := prof.Upstreams[idx]
	ids, lastErr := fetchUpstreamIDs(u.BaseURL, u.APIKey, u.Headers)
	if ids == nil {
		c.JSON(500, gin.H{"error": lastErr})
		return
	}
	seeds := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		seeds = append(seeds, map[string]interface{}{
			"id":            id,
			"contextWindow": uint32(128000),
			"maxTokens":     uint32(16384),
			"input":         []string{"text"},
		})
	}
	enriched, skipped, failed, warning := enrichModelsWithCatalog(seeds, prof)
	var entries []config.ModelEntry
	if b, err := json.Marshal(seeds); err == nil {
		_ = json.Unmarshal(b, &entries)
	}
	seen := map[string]bool{}
	for _, m := range prof.Upstreams[idx].Models {
		seen[m.ID] = true
	}
	for _, e := range entries {
		if strings.TrimSpace(e.ID) == "" || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		prof.Upstreams[idx].Models = append(prof.Upstreams[idx].Models, e)
	}
	cfg.Profiles[name] = prof
	if err := saveConfig(cfg); err != nil {
		c.JSON(500, gin.H{"error": "failed to save config: " + err.Error()})
		return
	}
	enrich := gin.H{"enriched": enriched, "skipped": skipped, "failed": failed}
	if warning != "" {
		enrich["warning"] = warning
	}
	c.JSON(200, gin.H{"models": ids, "enrich": enrich})
}

// fetchUpstreamUsage GETs {baseURL}/usage with bearer key and returns the
// upstream "usage" object (rolling/weekly/monthly windows). Any non-200
// status or unparsable body is an error: callers surface it instead of
// inventing numbers. User-Agent is required, Cloudflare 403s UA-less
// requests from opencode.ai frontends.
func fetchUpstreamUsage(baseURL, apiKey, userAgent string) (map[string]interface{}, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseUrl is empty")
	}
	u := strings.TrimRight(baseURL, "/") + "/usage"
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("usage endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("usage endpoint returned invalid JSON: %v", err)
	}
	usage, ok := decoded["usage"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("usage endpoint response has no usage object")
	}
	return usage, nil
}

// fetchUpstreamIDs tries baseURL/models then baseURL/v1/models with optional
// bearer key and headers. Returns nil ids + lastErr when all attempts fail.
func fetchUpstreamIDs(baseURL, apiKey string, headers map[string]string) ([]string, string) {
	if baseURL == "" {
		return nil, "baseUrl is empty"
	}
	client := &http.Client{Timeout: 5 * time.Second}
	urls := []string{strings.TrimRight(baseURL, "/") + "/models", strings.TrimRight(baseURL, "/") + "/v1/models"}
	var lastErr string
	for _, u := range urls {
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		for k, v := range headers {
			if k != "" {
				req.Header.Set(k, v)
			}
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Sprintf("upstream %d: %s", resp.StatusCode, string(body))
			continue
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(body, &parsed); err != nil {
			lastErr = err.Error()
			continue
		}
		var ids []string
		if data, ok := parsed["data"]; ok {
			if arr, ok := data.([]interface{}); ok {
				for _, v := range arr {
					switch vv := v.(type) {
					case string:
						ids = append(ids, vv)
					case map[string]interface{}:
						if id, ok := vv["id"].(string); ok {
							ids = append(ids, id)
						}
					}
				}
			}
		}
		if len(ids) == 0 {
			if m, ok := parsed["models"]; ok {
				if arr, ok := m.([]interface{}); ok {
					for _, v := range arr {
						if s, ok := v.(string); ok {
							ids = append(ids, s)
						}
					}
				}
			}
		}
		return ids, ""
	}
	return nil, lastErr
}

func handleFetchModels(c *gin.Context) {
	name := c.Param("name")
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	prof, ok := cfg.Profiles[name]
	if !ok {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	// 渠道定向拉取：?channel=name 用该渠道的 baseUrl/apiKey/headers 拉取，
	// enrich 后仅新增 id 合并入该渠道池并落盘；未知渠道 400。
	// 无参走下方旧路径（兼容，不写盘）。
	if channel := c.Query("channel"); channel != "" {
		handleFetchModelsForChannel(c, cfg, name, prof, channel)
		return
	}
	baseURL := prof.PrimaryBaseURL()
	apiKey := prof.PrimaryAPIKey()
	if baseURL == "" {
		c.JSON(200, gin.H{"models": []string{}, "enrich": gin.H{"enriched": 0, "skipped": 0, "failed": 0}})
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	urls := []string{strings.TrimRight(baseURL, "/") + "/models", strings.TrimRight(baseURL, "/") + "/v1/models"}
	var lastErr string
	for _, u := range urls {
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Sprintf("upstream %d: %s", resp.StatusCode, string(body))
			continue
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(body, &parsed); err != nil {
			lastErr = err.Error()
			continue
		}
		var ids []string
		if data, ok := parsed["data"]; ok {
			if arr, ok := data.([]interface{}); ok {
				for _, v := range arr {
					switch vv := v.(type) {
					case string:
						ids = append(ids, vv)
					case map[string]interface{}:
						if id, ok := vv["id"].(string); ok {
							ids = append(ids, id)
						}
					}
				}
			}
		}
		if len(ids) == 0 {
			if m, ok := parsed["models"]; ok {
				if arr, ok := m.([]interface{}); ok {
					for _, v := range arr {
						if s, ok := v.(string); ok {
							ids = append(ids, s)
						}
					}
				}
			}
		}
		if len(ids) == 0 {
			if arr, ok := parsed["data"].([]interface{}); ok && len(arr) == 0 {
				// empty
			}
		}
		// enrich: build default ModelEntry for each id, then enrich with catalog
		models := make([]map[string]interface{}, 0, len(ids))
		for _, id := range ids {
			m := map[string]interface{}{
				"id":            id,
				"contextWindow": uint32(128000),
				"maxTokens":     uint32(16384),
				"input":         []string{"text"},
			}
			models = append(models, m)
		}
		enriched, skipped, failed, warning := enrichModelsWithCatalog(models, prof)
		enrich := gin.H{"enriched": enriched, "skipped": skipped, "failed": failed}
		if warning != "" {
			enrich["warning"] = warning
		}
		// For compat, also return models as []string ids but include enrich stats
		// Check if caller expects full objects; we return ids as strings for backward compat, but also support objects
		c.JSON(200, gin.H{"models": ids, "enrich": enrich})
		return
	}
	c.JSON(500, gin.H{"error": lastErr})
}

func resolveModelsDevProvider(prof config.ProviderProfile) string {
	if prof.ModelsDevProvider != nil && *prof.ModelsDevProvider != "" {
		return *prof.ModelsDevProvider
	}
	if prof.Preset != nil {
		presetToDev := map[string]string{"openai": "openai", "anthropic": "anthropic", "google": "google", "deepseek": "deepseek", "xai": "xai", "moonshot": "moonshot", "qwen": "qwen", "cohere": "cohere", "mistral": "mistral", "azure": "azure"}
		if v, ok := presetToDev[*prof.Preset]; ok {
			return v
		}
	}
	return ""
}

func enrichModelsWithCatalog(models []map[string]interface{}, prof config.ProviderProfile) (enriched, skipped, failed int, warning string) {
	providerKey := resolveModelsDevProvider(prof)
	if providerKey == "" {
		return 0, len(models), 0, "no modelsDevProvider"
	}
	snap, _, snapWarn := catalog.Ensure()
	if snap.Empty() {
		if snapWarn == "" {
			snapWarn = fmt.Sprintf("catalog not found for %s", providerKey)
		}
		return 0, 0, len(models), snapWarn
	}
	for _, m := range models {
		id, _ := m["id"].(string)
		meta, ok := snap.LookupWithProvider(id, providerKey)
		if !ok {
			skipped++
			continue
		}
		if meta.ContextWindow != 0 {
			m["contextWindow"] = float64(meta.ContextWindow)
		}
		if meta.MaxTokens != 0 {
			m["maxTokens"] = float64(meta.MaxTokens)
		}
		m["reasoning"] = meta.Reasoning
		if len(meta.Input) > 0 {
			m["input"] = meta.Input
		}
		if meta.Name != "" {
			m["name"] = meta.Name
		}
		if meta.CostInput != 0 || meta.CostOutput != 0 || meta.CacheRead != 0 {
			m["cost"] = map[string]interface{}{
				"input":      meta.CostInput,
				"output":     meta.CostOutput,
				"cacheRead":  meta.CacheRead,
				"cacheWrite": float64(0),
			}
		}
		enriched++
	}
	if snapWarn != "" {
		warning = snapWarn
	}
	return enriched, skipped, failed, warning
}

func handlePutModels(c *gin.Context) {
	name := c.Param("name")
	var body struct {
		Models  []config.ModelEntry `json:"models"`
		Channel string              `json:"channel,omitempty"`
	}
	raw, _ := c.GetRawData()
	_ = json.Unmarshal(raw, &body)
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	prof, ok := cfg.Profiles[name]
	if !ok {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	if body.Channel == "" {
		c.JSON(400, gin.H{"error": "channel is required"})
		return
	}
	idx := ensureMutationChannel(&prof, body.Channel)
	if idx < 0 {
		c.JSON(400, gin.H{"error": fmt.Sprintf("unknown channel %q", body.Channel)})
		return
	}
	seen := map[string]bool{}
	for _, m := range body.Models {
		if strings.TrimSpace(m.ID) == "" {
			c.JSON(400, gin.H{"error": "model id must not be empty"})
			return
		}
		if seen[m.ID] {
			c.JSON(400, gin.H{"error": fmt.Sprintf("duplicate model id %q", m.ID)})
			return
		}
		seen[m.ID] = true
	}
	prof.Upstreams[idx].Models = body.Models
	cfg.Profiles[name] = prof
	if err := saveConfig(cfg); err != nil {
		c.JSON(500, gin.H{"error": "failed to save config: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "backup": nil, "enrich": gin.H{"enriched": 0}})
}

func handlePutExpose(c *gin.Context) {
	name := c.Param("name")
	var body struct {
		ModelIds []string `json:"modelIds"`
	}
	raw, _ := c.GetRawData()
	_ = json.Unmarshal(raw, &body)
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	prof, ok := cfg.Profiles[name]
	if !ok {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	channel := c.Query("channel")
	if channel == "" {
		c.JSON(400, gin.H{"error": "channel is required"})
		return
	}
	idx := ensureMutationChannel(&prof, channel)
	if idx < 0 {
		c.JSON(400, gin.H{"error": fmt.Sprintf("unknown channel %q", channel)})
		return
	}
	seen := map[string]bool{}
	for _, m := range prof.Upstreams[idx].Models {
		seen[m.ID] = true
	}
	for _, eid := range body.ModelIds {
		if !seen[eid] {
			c.JSON(400, gin.H{"error": fmt.Sprintf("exposedModels references unknown model %q in channel %q", eid, channel)})
			return
		}
	}
	prof.Upstreams[idx].ExposedModels = body.ModelIds
	cfg.Profiles[name] = prof
	if err := saveConfig(cfg); err != nil {
		c.JSON(500, gin.H{"error": "failed to save config: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "backup": nil})
}

func handlePutSpoof(c *gin.Context) {
	name := c.Param("name")
	var body struct {
		Spoof *string `json:"spoof"`
	}
	raw, _ := c.GetRawData()
	_ = json.Unmarshal(raw, &body)
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	prof, ok := cfg.Profiles[name]
	if !ok {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	// validate
	if body.Spoof != nil {
		v := *body.Spoof
		if v != "" && v != "claude-code" && v != "codex" && v != "gemini" {
			c.JSON(400, gin.H{"error": fmt.Sprintf("invalid spoof %q, must be one of '', 'claude-code', 'codex', 'gemini'", v)})
			return
		}
		if v == "" {
			prof.UserAgent = nil
		} else {
			s := v
			prof.UserAgent = &s
		}
	} else {
		prof.UserAgent = nil
	}
	cfg.Profiles[name] = prof
	if err := saveConfig(cfg); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "backup": nil})
}

func handleGetCredits(c *gin.Context) {
	name := c.Param("name")
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	prof, ok := cfg.Profiles[name]
	if !ok {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	baseURL := prof.BaseURL
	apiKey := prof.APIKey
	if len(prof.Upstreams) > 0 {
		if prof.Upstreams[0].BaseURL != "" {
			baseURL = prof.Upstreams[0].BaseURL
		}
		if prof.Upstreams[0].APIKey != "" {
			apiKey = prof.Upstreams[0].APIKey
		}
	}
	hasModels := false
	for i := range prof.Upstreams {
		if len(prof.Upstreams[i].Models) > 0 {
			hasModels = true
			break
		}
	}
	if baseURL == "" || !hasModels {
		c.JSON(200, gin.H{"balance": 0, "used": 0, "total": 0, "remaining": 0, "percent": 0, "raw": gin.H{}})
		return
	}
	// 真实回源：opencode 兼容网关的 {baseURL}/usage（zen/go 实测返回
	// rolling/weekly/monthly 用量百分比；Zen-only key 可能 403）。
	// 非 200/解析失败一律如实报错，前端显示错误+重试，不再编造数字。
	usage, err := fetchUpstreamUsage(baseURL, apiKey, resolveUserAgentValues(prof.UserAgent, cfg.Settings.Proxy.UserAgent))
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error(), "type": "upstream_error"})
		return
	}
	percent := 0.0
	if rolling, ok := usage["rolling"].(map[string]interface{}); ok {
		if p, ok := rolling["percent"].(float64); ok {
			percent = p
		}
	}
	c.JSON(200, gin.H{"balance": 0, "used": 0, "total": 0, "remaining": 0, "percent": percent, "usage": usage, "raw": gin.H{}})
}

// ProviderPresets is the single source of the static provider preset list,
// shared by GET /api/presets and `pi-switch preset`.
func ProviderPresets() []map[string]interface{} {
	return []map[string]interface{}{
		{"id": "openai", "name": "OpenAI", "description": "OpenAI API", "websiteUrl": "https://openai.com", "api": "openai-completions", "baseUrl": "https://api.openai.com/v1", "models": []string{"gpt-4o-mini", "gpt-4o", "o1"}},
		{"id": "anthropic", "name": "Anthropic", "description": "Anthropic API", "websiteUrl": "https://anthropic.com", "api": "anthropic-messages", "baseUrl": "https://api.anthropic.com", "models": []string{"claude-3-5-sonnet", "claude-3-opus"}},
		{"id": "google", "name": "Google", "description": "Google Gemini", "websiteUrl": "https://ai.google.dev", "api": "google-generative-ai", "baseUrl": "https://generativelanguage.googleapis.com/v1", "models": []string{"gemini-pro"}},
		{"id": "deepseek", "name": "DeepSeek", "description": "DeepSeek", "websiteUrl": "https://deepseek.com", "api": "openai-completions", "baseUrl": "https://api.deepseek.com/v1", "models": []string{"deepseek-chat"}},
	}
}

func handlePresets(c *gin.Context) {
	c.JSON(200, ProviderPresets())
}
func handlePresetDetail(c *gin.Context) {
	c.JSON(404, gin.H{"error": "not found"})
}
func handleDoctor(c *gin.Context) {
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	checks := []map[string]interface{}{
		{"ok": true, "msg": "config JSON is valid"},
		{"ok": len(cfg.Profiles) > 0, "msg": fmt.Sprintf("%d profile(s) configured", len(cfg.Profiles))},
	}
	c.JSON(200, checks)
}

func validateResponsesMode(p config.ProviderProfile) error {
	mode := p.ResponsesMode
	if mode == "" {
		mode = "auto"
	}
	api := p.API
	if mode == "passthrough" && api != "openai-responses" {
		return fmt.Errorf("responsesMode passthrough requires api openai-responses, got %s", api)
	}
	if mode == "convert" && api != "openai-completions" {
		return fmt.Errorf("responsesMode convert requires api openai-completions, got %s", api)
	}
	return nil
}

// isValidChannelName enforces the channel primary-key rule: required,
// 1-32 chars, letters/digits/'-'/"_" only (stable gateway id segment).
func isValidChannelName(name string) bool {
	if len(name) == 0 || len(name) > 32 {
		return false
	}
	for _, c := range name {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' {
			continue
		}
		return false
	}
	return true
}

// ensureMutationChannel resolves a named channel and upgrades a legacy single unnamed upstream to main.
func ensureMutationChannel(prof *config.ProviderProfile, channel string) int {
	if channel == "" {
		return -1
	}
	if idx := channelIndex(*prof, channel); idx >= 0 {
		return idx
	}
	if channel != "main" || len(prof.Upstreams) > 1 {
		return -1
	}
	if len(prof.Upstreams) == 0 {
		name := "main"
		prof.Upstreams = []config.Upstream{{
			Name:          &name,
			API:           prof.API,
			ResponsesMode: prof.ResponsesMode,
			BaseURL:       prof.BaseURL,
			APIKey:        prof.APIKey,
			Headers:       prof.Headers,
		}}
		return 0
	}
	if prof.ChannelName(0) != "" {
		return -1
	}
	name := "main"
	u := &prof.Upstreams[0]
	u.Name = &name
	if u.API == "" {
		u.API = prof.API
	}
	if u.ResponsesMode == "" {
		u.ResponsesMode = prof.ResponsesMode
	}
	if u.BaseURL == "" {
		u.BaseURL = prof.BaseURL
	}
	if u.APIKey == "" {
		u.APIKey = prof.APIKey
	}
	if u.Headers == nil {
		u.Headers = prof.Headers
	}
	return 0
}

// channelIndex returns the upstream index of the named channel, or -1.
func channelIndex(prof config.ProviderProfile, channel string) int {
	for i := range prof.Upstreams {
		if prof.ChannelName(i) == channel {
			return i
		}
	}
	return -1
}

func validateProviderProfile(p config.ProviderProfile) error {
	if p.BaseURL != "" && !strings.HasPrefix(p.BaseURL, "http://") && !strings.HasPrefix(p.BaseURL, "https://") {
		return fmt.Errorf("baseUrl must start with http:// or https://")
	}
	seenChannel := map[string]bool{}
	for _, u := range p.Upstreams {
		if u.BaseURL != "" && !strings.HasPrefix(u.BaseURL, "http://") && !strings.HasPrefix(u.BaseURL, "https://") {
			return fmt.Errorf("upstreams baseUrl must start with http:// or https://")
		}
		name := ""
		if u.Name != nil {
			name = *u.Name
		}
		if !isValidChannelName(name) {
			return fmt.Errorf("upstreams name %q invalid: required, 1-32 chars of [A-Za-z0-9-_]", name)
		}
		if seenChannel[name] {
			return fmt.Errorf("duplicate upstream name %q", name)
		}
		seenChannel[name] = true
		if err := config.ValidateUpstreamAPI(u, p); err != nil {
			return err
		}
		// 分区校验：池内 id 去重；暴露 id 必须归属本渠道池。
		poolSeen := map[string]bool{}
		for _, m := range u.Models {
			if strings.TrimSpace(m.ID) == "" {
				return fmt.Errorf("upstreams[%q] model id must not be empty", name)
			}
			if poolSeen[m.ID] {
				return fmt.Errorf("upstreams[%q] duplicate model id %q", name, m.ID)
			}
			poolSeen[m.ID] = true
		}
		for _, eid := range u.ExposedModels {
			if !poolSeen[eid] {
				return fmt.Errorf("upstreams[%q] exposedModels references unknown model %q", name, eid)
			}
		}
	}
	return nil
}

func handleValidate(c *gin.Context) {
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	issues := []map[string]interface{}{}
	allowedAPIs := map[string]bool{"openai-completions": true, "openai-responses": true, "anthropic-messages": true, "google-generative-ai": true}
	for name, prof := range cfg.Profiles {
		if prof.API == "" {
			issues = append(issues, map[string]interface{}{"level": "error", "path": fmt.Sprintf("profiles.%s.api", name), "message": "api required"})
		} else if !allowedAPIs[prof.API] {
			issues = append(issues, map[string]interface{}{"level": "error", "path": fmt.Sprintf("profiles.%s.api", name), "message": fmt.Sprintf("unsupported api %s", prof.API)})
		}
		if prof.BaseURL == "" && len(prof.Upstreams) == 0 {
			issues = append(issues, map[string]interface{}{"level": "error", "path": fmt.Sprintf("profiles.%s.baseUrl", name), "message": "baseUrl required"})
		}
		hasModels := false
		for _, u := range prof.Upstreams {
			if len(u.Models) > 0 {
				hasModels = true
				break
			}
		}
		if !hasModels {
			issues = append(issues, map[string]interface{}{"level": "warning", "path": fmt.Sprintf("profiles.%s.upstreams", name), "message": "no models"})
		}
		if prof.ModelsDevProvider != nil && *prof.ModelsDevProvider != "" {
			known := map[string]bool{"openai": true, "anthropic": true, "google": true, "deepseek": true, "xai": true, "moonshot": true, "qwen": true, "cohere": true, "mistral": true, "azure": true, "custom": true}
			if !known[*prof.ModelsDevProvider] {
				issues = append(issues, map[string]interface{}{"level": "warning", "path": fmt.Sprintf("profiles.%s.modelsDevProvider", name), "message": fmt.Sprintf("modelsDevProvider unknown: %s", *prof.ModelsDevProvider)})
			}
		}
		if err := validateResponsesMode(prof); err != nil {
			issues = append(issues, map[string]interface{}{"level": "error", "path": fmt.Sprintf("profiles.%s.responsesMode", name), "message": err.Error()})
		}
		if err := validateRetryFields(prof); err != nil {
			issues = append(issues, map[string]interface{}{"level": "error", "path": fmt.Sprintf("profiles.%s.retry", name), "message": err.Error()})
		}
		for idx, u := range prof.Upstreams {
			if err := config.ValidateUpstreamAPI(u, prof); err != nil {
				issues = append(issues, map[string]interface{}{"level": "error", "path": fmt.Sprintf("profiles.%s.upstreams[%d].api", name, idx), "message": err.Error()})
			}
		}
	}
	// failover check removed (transitional)
	if err := validateSettingsRetry(cfg.Settings); err != nil {
		issues = append(issues, map[string]interface{}{"level": "error", "path": "settings.proxy.retry", "message": err.Error()})
	}
	if len(issues) == 0 {
		// ensure at least empty array not null
		c.JSON(200, []interface{}{})
		return
	}
	c.JSON(200, issues)
}

func handleCcsProviders(c *gin.Context) { c.JSON(200, gin.H{"providers": []interface{}{}}) }
func handleCcsImport(c *gin.Context) {
	// Reporting ok with imported:0 for work that never happens is the defect
	// this endpoint must not reintroduce.
	c.JSON(501, notImplemented("ccswitch import"))
}
