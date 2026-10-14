package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/application"
	"github.com/heihei0299/pi-switch/internal/bootstrap"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/profile"
	"github.com/heihei0299/pi-switch/internal/protocol"
	providerusecase "github.com/heihei0299/pi-switch/internal/usecase/providers"
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
	if err := application.ValidateWritableConfig(cfg); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
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
		// The Native UI takes its api list and responsesMode rule from here
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

// respondProfileError maps a profile-domain error onto the status codes these
// endpoints already answered. Every arm matches by kind, so the order of the arms
// cannot misread a storage failure as a missing profile.
func respondProfileError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, profile.ErrProfileNotFound), errors.Is(err, providerusecase.ErrProfileNotFound):
		c.JSON(404, gin.H{"error": "not found"})
	case errors.Is(err, profile.ErrProfileExists), errors.Is(err, providerusecase.ErrProfileExists):
		c.JSON(400, gin.H{"error": "target exists"})
	case errors.Is(err, providerusecase.ErrPersistence):
		c.JSON(500, gin.H{"error": "failed to save config: " + err.Error()})
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
	// 校验（含 responsesMode 兼容性）、重名与落盘都在 Provider usecase 里（与 CLI
	// 共用一份实现），handler 只负责把它映射成 HTTP 错误码：落盘失败 500，
	// 其余 400。
	if _, err := bootstrap.NewProviderService(configPath()).CreateLegacy(c.Request.Context(), body.Name, prof); err != nil {
		if errors.Is(err, providerusecase.ErrPersistence) {
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
	if err := json.Unmarshal(raw, &body); err != nil {
		c.JSON(400, gin.H{"error": "invalid json"})
		return
	}
	var prof config.ProviderProfile
	if err := json.Unmarshal(body.Profile, &prof); err != nil {
		c.JSON(400, gin.H{"error": fmt.Sprintf("invalid profile: %v", err)})
		return
	}
	renameFrom := ""
	if body.RenameFrom != nil {
		renameFrom = *body.RenameFrom
	}
	if err := application.PutLegacyProvider(c.Request.Context(), configPath(), name, renameFrom, prof); err != nil {
		var validation *providerusecase.ValidationError
		if errors.As(err, &validation) {
			c.JSON(400, gin.H{"error": validation.Err.Error()})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true, "backup": nil})
}

func handleDeleteProfile(c *gin.Context) {
	name := c.Param("name")
	err := application.DeleteLegacyProvider(c.Request.Context(), configPath(), name)
	if errors.Is(err, providerusecase.ErrProfileNotFound) {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	if err != nil {
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
	if _, err := bootstrap.NewProviderService(configPath()).DuplicateLegacy(c.Request.Context(), name, body.As); err != nil {
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
	return application.FetchUpstreamUsage(baseURL, apiKey, userAgent)
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
	if baseURL == "" {
		c.JSON(200, gin.H{"models": []string{}, "enrich": gin.H{"enriched": 0, "skipped": 0, "failed": 0}})
		return
	}
	// 只读列出：拉取复用 fetchUpstreamIDs（渠道定向路径用同一原语），此处不写盘。
	ids, lastErr := profile.FetchUpstreamModelIDs(prof)
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
	err := bootstrap.NewProviderService(configPath()).SetModelsLegacy(c.Request.Context(), name, body.Channel, body.Models)
	if errors.Is(err, providerusecase.ErrProfileNotFound) {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	if errors.Is(err, providerusecase.ErrUnknownChannel) {
		c.JSON(400, gin.H{"error": fmt.Sprintf("unknown channel %q", body.Channel)})
		return
	}
	if err != nil {
		var validation *providerusecase.ValidationError
		if errors.As(err, &validation) {
			c.JSON(400, gin.H{"error": validation.Err.Error()})
			return
		}
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
	// Provider usecase reloads under the config write lock before changing exposure.
	channel := c.Query("channel")
	err := bootstrap.NewProviderService(configPath()).SetExposedModelsLegacy(c.Request.Context(), name, channel, body.ModelIds)
	if errors.Is(err, providerusecase.ErrProfileNotFound) {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		var validation *providerusecase.ValidationError
		if errors.As(err, &validation) || errors.Is(err, providerusecase.ErrUnknownChannel) {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
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
	err := bootstrap.NewProviderService(configPath()).SetUserAgentLegacy(c.Request.Context(), name, body.Spoof)
	if errors.Is(err, providerusecase.ErrProfileNotFound) {
		c.JSON(404, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		var validation *providerusecase.ValidationError
		if errors.As(err, &validation) {
			c.JSON(400, gin.H{"error": validation.Err.Error()})
			return
		}
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

// ProviderPresets retains the existing HTTP/CLI entry point during migration.
func ProviderPresets() []map[string]interface{} { return application.ProviderPresets() }

func handlePresets(c *gin.Context) {
	c.JSON(200, ProviderPresets())
}
func handlePresetDetail(c *gin.Context) {
	c.JSON(404, gin.H{"error": "not found"})
}
func handleDoctor(c *gin.Context) {
	checks, _ := application.DiagnoseConfig(configPath())
	c.JSON(200, checks)
}

func handleValidate(c *gin.Context) {
	_, issues := application.DiagnoseConfig(configPath())
	c.JSON(200, issues)
}

func handleCcsProviders(c *gin.Context) { c.JSON(200, gin.H{"providers": []interface{}{}}) }
func handleCcsImport(c *gin.Context) {
	// Reporting ok with imported:0 for work that never happens is the defect
	// this endpoint must not reintroduce.
	c.JSON(501, notImplemented("ccswitch import"))
}
