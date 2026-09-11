package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/profile"
	"github.com/heihei0299/pi-switch/internal/protocol"
)

func handleGetConfig(c *gin.Context) {
	cfg, src, err := config.LoadConfigAtPath(configPath())
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"source": src, "config": cfg})
}

func handlePutConfig(c *gin.Context) {
	raw, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	// Parse before persisting: writing the raw body would let an operator save
	// JSON the strict loader then rejects, and the writer would bypass the
	// migration and 0600 atomic save every other entry point uses.
	cfg, err := config.ParseConfig(raw)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid config: " + err.Error()})
		return
	}
	for name, prof := range cfg.Profiles {
		if prof.API == "" || prof.ResponsesMode == "" {
			continue
		}
		if err := protocol.ValidateResponsesMode(prof.API, prof.ResponsesMode); err != nil {
			c.JSON(400, gin.H{"error": fmt.Sprintf("profile %s: %s", name, err.Error())})
			return
		}
	}
	if err := config.SaveAtPath(cfg, configPath()); err != nil {
		c.JSON(500, gin.H{"error": "failed to save config: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func handleGetState(c *gin.Context) {
	cfg, ok := loadConfigOrWrite(c)
	if !ok {
		return
	}
	c.JSON(200, gin.H{
		"current":  cfg.Current,
		"profiles": cfg.Profiles,
		"settings": cfg.Settings,
		// The WebUI takes its api list and responsesMode rule from here
		// (internal/protocol is the single source), never from a local copy.
		"protocol": gin.H{"apis": protocol.Capabilities()},
	})
}

func handleListProfiles(c *gin.Context) {
	cfg, ok := loadConfigOrWrite(c)
	if !ok {
		return
	}
	c.JSON(200, cfg.Profiles)
}

func handleGetProfile(c *gin.Context) {
	name := c.Param("name")
	cfg, ok := loadConfigOrWrite(c)
	if !ok {
		return
	}
	prof, ok := cfg.Profiles[name]
	if !ok {
		c.JSON(404, gin.H{"error": fmt.Sprintf("unknown profile '%s'", name)})
		return
	}
	c.JSON(200, gin.H{"name": name, "profile": prof, "providerId": name})
}

// isPersistError distinguishes a failed write from a rejected input, so the
// handler can keep answering 400 for validation and 500 for storage. It matches
// the kind rather than the syscall error types, so a wrapped or future persist
// error cannot silently downgrade 500 to 400.
func isPersistError(err error) bool {
	return errors.Is(err, profile.ErrPersistFailed)
}

// respondProfileError maps a profile-domain error onto the status codes these
// endpoints already answered. Every arm matches by kind, so the order of the arms
// cannot misread a storage failure as a missing profile.
func respondProfileError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, profile.ErrProfileNotFound):
		c.JSON(404, gin.H{"error": "not found"})
	case errors.Is(err, profile.ErrProfileExists):
		c.JSON(400, gin.H{"error": "target exists"})
	case errors.Is(err, profile.ErrPersistFailed), errors.Is(err, profile.ErrUpstreamFetchFailed):
		c.JSON(500, gin.H{"error": err.Error()})
	default:
		c.JSON(400, gin.H{"error": err.Error()})
	}
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
						if err := protocol.ValidateResponsesMode(api, mode); err != nil {
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
								if err := protocol.ValidateResponsesMode(api, mode); err != nil {
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
					if err := protocol.ValidateResponsesMode(api, mode); err != nil {
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
	if err := profile.CreateProfile(body.Name, prof); err != nil {
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
						if err := protocol.ValidateResponsesMode(api, mode); err != nil {
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
								if err := protocol.ValidateResponsesMode(api, mode); err != nil {
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
					if err := protocol.ValidateResponsesMode(api, mode); err != nil {
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
	if err := profile.ValidateResponsesMode(prof); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := profile.ValidateProviderProfile(prof); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := config.ValidateProviderRetry(prof); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	cfg, ok := loadConfigOrWrite(c)
	if !ok {
		return
	}
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
	cfg, ok := loadConfigOrWrite(c)
	if !ok {
		return
	}
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

func handleDuplicateProfile(c *gin.Context) {
	name := c.Param("name")
	var body struct {
		As string `json:"as"`
	}
	raw, _ := c.GetRawData()
	_ = json.Unmarshal(raw, &body)
	if err := profile.DuplicateProfile(name, body.As); err != nil {
		respondProfileError(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func handleTestProfile(c *gin.Context) {
	name := c.Param("name")
	cfg, ok := loadConfigOrWrite(c)
	if !ok {
		return
	}
	prof, ok := cfg.Profiles[name]
	if !ok {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	success, message, ms := profile.TestProfileUpstream(prof)
	c.JSON(200, gin.H{"success": success, "message": message, "responseTimeMs": ms})
}

func handleFetchModelsForChannel(c *gin.Context, name, channel string) {
	ids, counts, err := profile.FetchChannelModels(name, channel)
	if err != nil {
		respondProfileError(c, err)
		return
	}
	enrich := gin.H{"enriched": counts.Enriched, "skipped": counts.Skipped, "failed": counts.Failed}
	if counts.Warning != "" {
		enrich["warning"] = counts.Warning
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

func handleFetchModels(c *gin.Context) {
	name := c.Param("name")
	cfg, ok := loadConfigOrWrite(c)
	if !ok {
		return
	}
	prof, ok := cfg.Profiles[name]
	if !ok {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	// 渠道定向拉取：?channel=name 用该渠道的 baseUrl/apiKey/headers 拉取，
	// enrich 后仅新增 id 合并入该渠道池并落盘；未知渠道 400。
	// 无参走下方旧路径（兼容，不写盘）。
	if channel := c.Query("channel"); channel != "" {
		handleFetchModelsForChannel(c, name, channel)
		return
	}
	baseURL := prof.PrimaryBaseURL()
	apiKey := prof.PrimaryAPIKey()
	if baseURL == "" {
		c.JSON(200, gin.H{"models": []string{}, "enrich": gin.H{"enriched": 0, "skipped": 0, "failed": 0}})
		return
	}
	// 只读列出：拉取复用 fetchUpstreamIDs（渠道定向路径用同一原语），此处不写盘。
	ids, lastErr := profile.FetchUpstreamIDs(baseURL, apiKey, nil)
	if ids == nil {
		c.JSON(500, gin.H{"error": lastErr})
		return
	}
	models := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		models = append(models, map[string]interface{}{
			"id":            id,
			"contextWindow": uint32(128000),
			"maxTokens":     uint32(16384),
			"input":         []string{"text"},
		})
	}
	enriched, skipped, failed, warning := profile.EnrichModelsWithCatalog(models, prof)
	enrich := gin.H{"enriched": enriched, "skipped": skipped, "failed": failed}
	if warning != "" {
		enrich["warning"] = warning
	}
	c.JSON(200, gin.H{"models": ids, "enrich": enrich})
}

func handlePutModels(c *gin.Context) {
	name := c.Param("name")
	var body struct {
		Models  []config.ModelEntry `json:"models"`
		Channel string              `json:"channel,omitempty"`
	}
	raw, _ := c.GetRawData()
	_ = json.Unmarshal(raw, &body)
	cfg, ok := loadConfigOrWrite(c)
	if !ok {
		return
	}
	prof, ok := cfg.Profiles[name]
	if !ok {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	if body.Channel == "" {
		c.JSON(400, gin.H{"error": "channel is required"})
		return
	}
	idx := profile.EnsureMutationChannel(&prof, body.Channel)
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
	// prof 由 SetExposedModels 内部加载，handler 只负责取 channel 与映射错误。
	channel := c.Query("channel")
	if err := profile.SetExposedModels(name, channel, body.ModelIds); err != nil {
		respondProfileError(c, err)
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
	cfg, ok := loadConfigOrWrite(c)
	if !ok {
		return
	}
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
	cfg, ok := loadConfigOrWrite(c)
	if !ok {
		return
	}
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
		{"id": "openai", "name": "OpenAI", "description": "OpenAI API", "websiteUrl": "https://openai.com", "api": protocol.OpenAIChat, "baseUrl": "https://api.openai.com/v1", "models": []string{"gpt-4o-mini", "gpt-4o", "o1"}},
		{"id": "anthropic", "name": "Anthropic", "description": "Anthropic API", "websiteUrl": "https://anthropic.com", "api": protocol.AnthropicMessages, "baseUrl": "https://api.anthropic.com", "models": []string{"claude-3-5-sonnet", "claude-3-opus"}},
		{"id": "google", "name": "Google", "description": "Google Gemini", "websiteUrl": "https://ai.google.dev", "api": protocol.GoogleGenerativeAI, "baseUrl": "https://generativelanguage.googleapis.com/v1", "models": []string{"gemini-pro"}},
		{"id": "deepseek", "name": "DeepSeek", "description": "DeepSeek", "websiteUrl": "https://deepseek.com", "api": protocol.OpenAIChat, "baseUrl": "https://api.deepseek.com/v1", "models": []string{"deepseek-chat"}},
	}
}

func handlePresets(c *gin.Context) {
	c.JSON(200, ProviderPresets())
}
func handlePresetDetail(c *gin.Context) {
	c.JSON(404, gin.H{"error": "not found"})
}
func handleDoctor(c *gin.Context) {
	cfg, _, err := config.LoadConfigAtPath(configPath())
	if err != nil {
		// The first check below claims the config JSON is valid, so a broken file
		// must flip that check instead of being reported as healthy.
		c.JSON(200, []map[string]interface{}{
			{"ok": false, "msg": "config unreadable: " + err.Error()},
		})
		return
	}
	checks := []map[string]interface{}{
		{"ok": true, "msg": "config JSON is valid"},
		{"ok": len(cfg.Profiles) > 0, "msg": fmt.Sprintf("%d profile(s) configured", len(cfg.Profiles))},
	}
	c.JSON(200, checks)
}

func handleValidate(c *gin.Context) {
	cfg, _, err := config.LoadConfigAtPath(configPath())
	if err != nil {
		c.JSON(200, []map[string]interface{}{
			{"level": "error", "path": "config", "message": err.Error()},
		})
		return
	}
	issues := []map[string]interface{}{}
	for name, prof := range cfg.Profiles {
		if prof.API == "" {
			issues = append(issues, map[string]interface{}{"level": "error", "path": fmt.Sprintf("profiles.%s.api", name), "message": "api required"})
		} else if !protocol.IsKnown(prof.API) {
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
		if err := profile.ValidateResponsesMode(prof); err != nil {
			issues = append(issues, map[string]interface{}{"level": "error", "path": fmt.Sprintf("profiles.%s.responsesMode", name), "message": err.Error()})
		}
		if err := config.ValidateProviderRetry(prof); err != nil {
			issues = append(issues, map[string]interface{}{"level": "error", "path": fmt.Sprintf("profiles.%s.retry", name), "message": err.Error()})
		}
		for idx, u := range prof.Upstreams {
			if err := config.ValidateUpstreamAPI(u, prof); err != nil {
				issues = append(issues, map[string]interface{}{"level": "error", "path": fmt.Sprintf("profiles.%s.upstreams[%d].api", name, idx), "message": err.Error()})
			}
		}
	}
	// failover check removed (transitional)
	if err := config.ValidateSettingsRetry(cfg.Settings); err != nil {
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
