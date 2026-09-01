package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/gateway"
	"github.com/heihei0299/pi-switch/internal/limit"
	"github.com/heihei0299/pi-switch/internal/scan"
	"github.com/heihei0299/pi-switch/internal/store"
)

func configPath() string {
	if p := os.Getenv("PI_SWITCH_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/pi-switch-config.json"
	}
	return filepath.Join(home, ".pi-switch", "config.json")
}

func NewProxyRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.POST("/v1/chat/completions", handleChatCompletions)
	r.POST("/v1/completions", handleChatCompletions)
	r.POST("/v1/responses", handleChatCompletions)
	r.POST("/v1/messages", handleChatCompletions)
	r.GET("/v1/models", handleModels)
	return r
}

func NewMgmtRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/", func(c *gin.Context) {
		c.Data(200, "text/html; charset=utf-8", []byte(`<html><body style="font-family:system-ui;padding:32px"><h1>pi-switch Go</h1><p>embed.FS placeholder for webui/dist</p><p style="color:#888">PROTOTYPE — will be replaced by webui/dist</p></body></html>`))
	})
	r.GET("/api/config", handleGetConfig)
	r.PUT("/api/config", handlePutConfig)
	r.GET("/api/stats", handleStats)
	r.GET("/api/profiles", handleGetConfig)
	r.PUT("/api/gateway/publish", handleGatewayPublish)
	r.POST("/api/gateway/publish", handleGatewayPublish)
	r.NoRoute(func(c *gin.Context) { c.JSON(404, gin.H{"error": "not found"}) })
	return r
}

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
	// validate is json
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		c.JSON(400, gin.H{"error": "invalid json"})
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	_ = os.Rename(tmp, path)
	c.JSON(200, gin.H{"ok": true})
}

func handleGatewayPublish(c *gin.Context) {
	raw, err := c.GetRawData()
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	var body map[string]interface{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			c.JSON(400, gin.H{"error": "invalid json"})
			return
		}
	}
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	// if body empty or doesn't contain providers key, compute proposed
	var toPublish map[string]interface{}
	if body != nil && len(body) > 0 {
		// if body contains "providers" wrapping? spec says PUT /api/gateway/publish writes models.json providers[providerPrefix]
		// support both: if body has "providers" or is the gateway entry itself
		if _, ok := body["providers"]; ok {
			// unexpected: write as is
			toPublish = body
			// fallback: if body is whole models.json, extract?
		} else if _, ok := body["api"]; ok {
			// body is the gateway entry itself
			toPublish = body
		} else {
			toPublish = gateway.BuildProposedGatewayEntry(cfg)
		}
	} else {
		toPublish = gateway.BuildProposedGatewayEntry(cfg)
	}
	// if toPublish is like providers wrapper, handle Publish differently
	if _, ok := toPublish["providers"]; ok {
		path := gateway.ModelsPath()
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		b, _ := json.MarshalIndent(toPublish, "", "  ")
		tmp := path + ".tmp"
		_ = os.WriteFile(tmp, append(b, '\n'), 0644)
		_ = os.Rename(tmp, path)
		c.JSON(200, gin.H{"ok": true})
		return
	}
	if err := gateway.Publish(cfg, toPublish); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func handleModels(c *gin.Context) {
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	data := []interface{}{}
	seen := map[string]bool{}
	for name, prof := range cfg.Profiles {
		// derive exposed models: if ExposedModels non-empty use that, else Models ids
		exposed := prof.ExposedModels
		if len(exposed) == 0 {
			for _, m := range prof.Models {
				exposed = append(exposed, m.ID)
			}
		}
		for _, mid := range exposed {
			id := name + "/" + mid
			if seen[id] {
				continue
			}
			seen[id] = true
			data = append(data, map[string]interface{}{"id": id, "object": "model", "owned_by": name})
		}
	}
	c.JSON(200, gin.H{"object": "list", "data": data})
}

// --- proxy helpers ---

func isNonProxy(cfg config.PiSwitchConfig, name string) bool {
	// in Go, no proxy flag; treat all as non-proxy
	_, ok := cfg.Profiles[name]
	return ok
}
func exposes(cfg config.PiSwitchConfig, name, model string) bool {
	prof, ok := cfg.Profiles[name]
	if !ok {
		return false
	}
	// check ExposedModels first, then Models
	if len(prof.ExposedModels) > 0 {
		for _, m := range prof.ExposedModels {
			if m == model {
				return true
			}
		}
		return false
	}
	for _, m := range prof.Models {
		if m.ID == model {
			return true
		}
	}
	return false
}
func resolveRoute(cfg config.PiSwitchConfig, requested string) ([]string, string) {
	if strings.Contains(requested, "/") {
		parts := strings.SplitN(requested, "/", 2)
		prefix, rest := parts[0], parts[1]
		if isNonProxy(cfg, prefix) && exposes(cfg, prefix, rest) {
			profiles := []string{prefix}
			for _, fo := range cfg.Settings.Proxy.Failover {
				if fo != prefix && isNonProxy(cfg, fo) && exposes(cfg, fo, rest) && !contains(profiles, fo) {
					profiles = append(profiles, fo)
				}
			}
			return profiles, rest
		}
	}
	// bare fallback: collect failover order first then rest
	profiles := []string{}
	for _, fo := range cfg.Settings.Proxy.Failover {
		if isNonProxy(cfg, fo) && exposes(cfg, fo, requested) && !contains(profiles, fo) {
			profiles = append(profiles, fo)
		}
	}
	for name := range cfg.Profiles {
		if isNonProxy(cfg, name) && exposes(cfg, name, requested) && !contains(profiles, name) {
			profiles = append(profiles, name)
		}
	}
	return profiles, requested
}
func contains(arr []string, s string) bool {
	for _, v := range arr {
		if v == s {
			return true
		}
	}
	return false
}

func conversationIDFrom(headers http.Header, body map[string]interface{}, source string) (string, string) {
	if source == "off" {
		return "unlabeled", ""
	}
	// try header/body first
	var id, name string
	if v := headers.Get("x-conversation-id"); v != "" {
		id = v
	} else if v := headers.Get("x-opencode-session"); v != "" {
		id = v
	} else if v, ok := body["conversation_id"].(string); ok && v != "" {
		id = v
	}
	if v := headers.Get("x-conversation-name"); v != "" {
		// decode percent-encoding (spec: non-Latin1 percent-encode, decode back)
		if dec, err := url.PathUnescape(v); err == nil {
			// only take if valid UTF-8? PathUnescape already validates
			// heuristic: if dec contains replacement char, keep raw
			if strings.Contains(dec, string('\uFFFD')) {
				name = v
			} else {
				name = strings.ReplaceAll(dec, "\r", " ")
				name = strings.ReplaceAll(name, "\n", " ")
				name = strings.ReplaceAll(name, "\t", " ")
				name = strings.TrimSpace(name)
			}
		} else {
			name = strings.ReplaceAll(v, "\r", " ")
			name = strings.ReplaceAll(name, "\n", " ")
			name = strings.ReplaceAll(name, "\t", " ")
			name = strings.TrimSpace(name)
		}
	}
	if id != "" {
		return id, name
	}
	if source == "proxy" {
		return "unlabeled", ""
	}
	// sessionScan virtual fallback
	// scan sessions
	sessions := scan.Scan()
	// we need model and time to match; body model?
	model, _ := body["model"].(string)
	// also prompt_tokens hint? not needed
	nowStr := time.Now().Format(time.RFC3339)
	nowMs := time.Now().UnixMilli()
	entryTs := nowStr
	// straw entry for matching
	entryModel := model
	if idx := strings.LastIndex(entryModel, "/"); idx >= 0 {
		entryModel = entryModel[idx+1:]
	}
	bestID := ""
	bestTitle := ""
	bestDiff := int64(999999999)
	var promptHint *uint64
	if pt, ok := body["prompt_tokens"].(float64); ok {
		u := uint64(pt)
		promptHint = &u
	}
	for _, sess := range sessions {
		sessModel := ""
		if sess.Model != nil {
			sessModel = *sess.Model
			if idx := strings.LastIndex(sessModel, "/"); idx >= 0 {
				sessModel = sessModel[idx+1:]
			}
		}
		// model check
		if entryModel != "" && sessModel != "" && entryModel != sessModel {
			continue
		}
		if entryModel == "" {
			continue
		}
		if sess.LastActiveAt == nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, *sess.LastActiveAt)
		if err != nil {
			continue
		}
		sessMs := t.UnixMilli()
		entryMs, _ := time.Parse(time.RFC3339, entryTs)
		diff := entryMs.UnixMilli() - sessMs
		if diff < 0 {
			diff = -diff
		}
		if diff > 2000 {
			continue
		}
		// 5min window check
		if nowMs-sessMs > 5*60*1000 || sessMs > nowMs+2000 {
			continue
		}
		var pdiff uint64 = 9999999
		if promptHint != nil && sess.PromptTokensHint != nil {
			if *promptHint > *sess.PromptTokensHint {
				pdiff = *promptHint - *sess.PromptTokensHint
			} else {
				pdiff = *sess.PromptTokensHint - *promptHint
			}
		} else if promptHint != nil || sess.PromptTokensHint != nil {
			// one has hint one doesn't: still consider but with high diff
			pdiff = 5000
		} else {
			pdiff = 0
		}
		// prefer smaller prompt diff then time diff
		score := int64(pdiff)*10000 + diff
		if score < bestDiff {
			bestDiff = score
			bestID = sess.ID
			bestTitle = sess.Title
		}
	}
	if bestID != "" {
		return bestID, bestTitle
	}
	return "unlabeled", ""
}

func findModelEntry(prof config.ProviderProfile, realModel string) *config.ModelEntry {
	for i := range prof.Models {
		if prof.Models[i].ID == realModel {
			return &prof.Models[i]
		}
	}
	return nil
}

func handleChatCompletions(c *gin.Context) {
	start := time.Now()
	raw, _ := io.ReadAll(c.Request.Body)
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		body = map[string]interface{}{}
	}
	// keep raw len for limit
	rawLen := len(raw)
	requestedModel, _ := body["model"].(string)
	if requestedModel == "" {
		requestedModel = "gpt-4o-mini"
	}
	candidates, realModel := resolveRoute(cfg, requestedModel)
	if len(candidates) == 0 {
		// still try current? fallback to first profile if single
		if cfg.Current != nil {
			if prof, ok := cfg.Profiles[*cfg.Current]; ok {
				candidates = []string{*cfg.Current}
				// ensure realModel is as requested stripped?
				realModel = requestedModel
				if strings.Contains(requestedModel, "/") {
					parts := strings.SplitN(requestedModel, "/", 2)
					if parts[0] == *cfg.Current {
						realModel = parts[1]
					}
				}
				_ = prof
			}
		}
	}
	if len(candidates) == 0 {
		c.JSON(502, gin.H{"error": gin.H{"message": fmt.Sprintf("No upstream exposes model '%s'", requestedModel), "type": "no_route"}})
		return
	}
	// conversation handling
	convID, convName := conversationIDFrom(c.Request.Header, body, cfg.Settings.ConversationSource)
	// attempt failover
	var lastErr string
	var lastStatus int = 502
	var successResp []byte
	var successHeaders http.Header
	var successProvider string
	var successModelEntry *config.ModelEntry
	for _, name := range candidates {
		prof, ok := cfg.Profiles[name]
		if !ok {
			continue
		}
		base := prof.PrimaryBaseURL()
		if base == "" {
			lastErr = "missing baseUrl"
			continue
		}
		// limit clamp
		modelEntry := findModelEntry(prof, realModel)
		if modelEntry == nil {
			// fallback default
			modelEntry = &config.ModelEntry{ID: realModel, ContextWindow: 128000, MaxTokens: 16384}
		}
		successModelEntry = modelEntry
		// clone body for this attempt
		bcopy := cloneMap(body)
		bcopy["model"] = realModel
		// clamp logic: estimate and rewrite max_tokens / max_output_tokens / max_completion_tokens
		for _, key := range []string{"max_tokens", "max_output_tokens", "max_completion_tokens"} {
			if v, ok := bcopy[key]; ok {
				var req int
				switch vv := v.(type) {
				case float64:
					req = int(vv)
				case int:
					req = vv
				default:
					continue
				}
				reqCopy := req
				clamped := limit.ClampMaxTokens(modelEntry.ContextWindow, modelEntry.MaxTokens, rawLen, &reqCopy)
				if clamped != req {
					bcopy[key] = float64(clamped)
				}
			}
		}
		// if no requested but we still might want to clamp? spec says clamp when requested exceeds; we only rewrite when present

		// build upstream url
		u := strings.TrimRight(base, "/")
		// which path? original request path is /v1/chat/completions
		origPath := c.Request.URL.Path
		if origPath == "" {
			origPath = "/v1/chat/completions"
		}
		// ensure we forward to same suffix but on upstream base
		// base already includes /v1 maybe, so just join origPath's last segments?
		// Simplified: if base ends with /v1, append /chat/completions
		if strings.HasSuffix(u, "/v1") {
			u = u + strings.TrimPrefix(origPath, "/v1")
		} else {
			u = u + origPath
		}

		// headers
		apiKey := prof.PrimaryAPIKey()
		headers := prof.PrimaryHeaders()
		// build request
		bbytes, _ := json.Marshal(bcopy)
		req, err := http.NewRequest("POST", u, bytes.NewReader(bbytes))
		if err != nil {
			lastErr = err.Error()
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		// custom headers from upstream / profile
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		// forward User-Agent if present?
		if ua := c.Request.Header.Get("User-Agent"); ua != "" {
			req.Header.Set("User-Agent", ua)
		} else {
			req.Header.Set("User-Agent", "curl/8.5.0")
		}
		// forward conversation headers transparently? not needed for upstream
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err.Error()
			lastStatus = 502
			// log failure then continue
			logRequest(name, realModel, false, 0, 0, 0, 0, nil, convID, convName, time.Since(start).Milliseconds(), 502, lastErr)
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 500 && resp.StatusCode <= 599 {
			lastErr = fmt.Sprintf("HTTP %d", resp.StatusCode)
			lastStatus = resp.StatusCode
			logRequest(name, realModel, false, 0, 0, 0, 0, nil, convID, convName, time.Since(start).Milliseconds(), resp.StatusCode, lastErr)
			continue
		}
		if resp.StatusCode >= 400 {
			// non-retryable: return directly (except 429 maybe retry, but spec says 5xx only)
			// For 429, treat as retryable? spec mentions 5xx chain, keep 429 as retryable
			if resp.StatusCode == 429 {
				lastErr = fmt.Sprintf("HTTP %d", resp.StatusCode)
				lastStatus = resp.StatusCode
				logRequest(name, realModel, false, 0, 0, 0, 0, nil, convID, convName, time.Since(start).Milliseconds(), resp.StatusCode, lastErr)
				continue
			}
			// 4xx non-retry: surface
			c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
			// log as failure? but not retried
			logRequest(name, realModel, false, 0, 0, 0, 0, nil, convID, convName, time.Since(start).Milliseconds(), resp.StatusCode, string(respBody))
			return
		}
		// success
		successResp = respBody
		successHeaders = resp.Header
		successProvider = name
		lastStatus = resp.StatusCode
		break
	}
	if successResp == nil {
		if lastErr == "" {
			lastErr = "All upstream attempts failed"
		}
		c.JSON(lastStatus, gin.H{"error": gin.H{"message": lastErr, "type": "failover_exhausted"}})
		return
	}
	// parse usage and cost, log
	var respObj map[string]interface{}
	_ = json.Unmarshal(successResp, &respObj)
	usagePrompt, usageCompletion, usageCached, usageReasoning := extractUsage(respObj)
	cost := computeCost(successModelEntry, usagePrompt, usageCompletion, usageCached)
	latMs := time.Since(start).Milliseconds()
	logRequest(successProvider, realModel, true, usagePrompt, usageCompletion, usageCached, usageReasoning, cost, convID, convName, latMs, lastStatus, "")

	// forward headers (keep content-type)
	for k, vv := range successHeaders {
		for _, v := range vv {
			if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Connection") {
				continue
			}
			c.Header(k, v)
		}
	}
	ct := successHeaders.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}
	c.Data(lastStatus, ct, successResp)
}

func cloneMap(m map[string]interface{}) map[string]interface{} {
	b, _ := json.Marshal(m)
	var out map[string]interface{}
	_ = json.Unmarshal(b, &out)
	return out
}

func extractUsage(resp map[string]interface{}) (prompt, completion, cached, reasoning int) {
	usage, ok := resp["usage"].(map[string]interface{})
	if !ok {
		return 0, 0, 0, 0
	}
	if v, ok := usage["prompt_tokens"].(float64); ok {
		prompt = int(v)
	}
	if v, ok := usage["completion_tokens"].(float64); ok {
		completion = int(v)
	}
	if v, ok := usage["total_tokens"].(float64); ok && prompt == 0 && completion == 0 {
		// ignore
		_ = v
	}
	if v, err := getNested(usage, "prompt_tokens_details", "cached_tokens"); err == nil {
		if f, ok := v.(float64); ok {
			cached = int(f)
		}
	}
	// alternative flat cached_tokens?
	if c, ok := usage["cached_tokens"].(float64); ok && cached == 0 {
		cached = int(c)
	}
	if v, err := getNested(usage, "completion_tokens_details", "reasoning_tokens"); err == nil {
		if f, ok := v.(float64); ok {
			reasoning = int(f)
		}
	}
	// also output_tokens_details
	if reasoning == 0 {
		if v, err := getNested(usage, "output_tokens_details", "reasoning_tokens"); err == nil {
			if f, ok := v.(float64); ok {
				reasoning = int(f)
			}
		}
	}
	return
}
func getNested(m map[string]interface{}, keys ...string) (interface{}, error) {
	cur := interface{}(m)
	for _, k := range keys {
		if mp, ok := cur.(map[string]interface{}); ok {
			cur = mp[k]
		} else {
			return nil, fmt.Errorf("not found")
		}
	}
	return cur, nil
}

func computeCost(entry *config.ModelEntry, prompt, completion, cached int) *float64 {
	if entry == nil || entry.Cost == nil {
		return nil
	}
	inputPrice := entry.Cost.Input
	outputPrice := entry.Cost.Output
	cacheReadPrice := entry.Cost.CacheRead
	// cost = (prompt-cached)*input + cached*cacheRead + completion*output divided by 1M
	promptNonCached := prompt - cached
	if promptNonCached < 0 {
		promptNonCached = 0
	}
	cost := float64(promptNonCached)*inputPrice/1_000_000 + float64(cached)*cacheReadPrice/1_000_000 + float64(completion)*outputPrice/1_000_000
	return &cost
}

func logRequest(provider, model string, success bool, prompt, completion, cached, reasoning int, cost *float64, convID, convName string, latency int64, status int, errMsg string) {
	db, err := store.GetDB()
	if err != nil {
		return
	}
	ts := time.Now().Format(time.RFC3339)
	succ := 0
	if success {
		succ = 1
	}
	var costVal interface{}
	if cost != nil {
		costVal = *cost
	} else {
		costVal = nil
	}
	// ensure reasoning column? already
	_, _ = db.Exec(`INSERT INTO requests(ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id,conversation_name,latency_ms) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		ts, provider, model, succ, prompt, completion, cached, reasoning, costVal, convID, convName, latency)
	_ = status
	_ = errMsg
	_ = math.Ceil // keep import
}

func handleStats(c *gin.Context) {
	groupBy := c.Query("groupBy")
	windowFrom := c.Query("from")
	windowTo := c.Query("to")
	// simple window not implemented; ignore
	_ = windowFrom
	_ = windowTo
	db, err := store.GetDB()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if groupBy == "conversation" {
		// return by_conversation aggregation
		cfg, _, _ := config.LoadConfigAtPath(configPath())
		source := cfg.Settings.ConversationSource
		rows, _ := db.Query(`SELECT ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id,conversation_name FROM requests ORDER BY id DESC`)
		if rows != nil {
			defer rows.Close()
			type convAgg struct {
				ID         string
				Name       *string
				Requests   int
				Input      int
				Output     int
				Cached     int
				Reasons    int
				Cost       *float64
				CostUnknown int
				LastActive *string
			}
			agg := map[string]*convAgg{}
			for rows.Next() {
				var ts, provider, model, convID, convName sql.NullString
				var succ, pt, ct, cached, reasoning sql.NullInt64
				var cost sql.NullFloat64
				_ = rows.Scan(&ts, &provider, &model, &succ, &pt, &ct, &cached, &reasoning, &cost, &convID, &convName)
				effID, effName := effectiveConversationID(convID, convName, provider, model, ts, source)
				a, ok := agg[effID]
				if !ok {
					a = &convAgg{ID: effID}
					if effName != "" {
						n := effName
						a.Name = &n
					}
					agg[effID] = a
				}
				a.Requests++
				if ts.Valid {
					if a.LastActive == nil || ts.String > *a.LastActive {
						s := ts.String
						a.LastActive = &s
					}
				}
				if succ.Valid && succ.Int64 == 1 {
					if pt.Valid {
						a.Input += int(pt.Int64)
					}
					if ct.Valid {
						a.Output += int(ct.Int64)
					}
					if cached.Valid {
						a.Cached += int(cached.Int64)
					}
					if reasoning.Valid {
						a.Reasons += int(reasoning.Int64)
					}
					if cost.Valid {
						if a.Cost == nil {
							v := cost.Float64
							a.Cost = &v
						} else {
							*a.Cost += cost.Float64
						}
					} else {
						// only count unknown when we had usage? per spec only when usage parsed; simplify: prompt present => unknown
						if pt.Valid {
							a.CostUnknown++
						}
					}
					if effName != "" && a.Name != nil && *a.Name == "" {
						n := effName
						a.Name = &n
					} else if effName != "" {
						n := effName
						a.Name = &n
					}
				}
			}
			list := []interface{}{}
			for _, v := range agg {
				rate := "-"
				if v.Input > 0 {
					if v.Cached == 0 {
						rate = "0.0%"
					} else {
						rate = fmt.Sprintf("%.1f%%", float64(v.Cached)/float64(v.Input)*100)
					}
				}
				m := map[string]interface{}{
					"conversationId": v.ID,
					"requests":       v.Requests,
					"inputTokens":    v.Input,
					"outputTokens":   v.Output,
					"cachedTokens":   v.Cached,
					"reasoningTokens": v.Reasons,
					"cacheRate":      rate,
					"lastActive":     v.LastActive,
				}
				if v.Name != nil {
					m["name"] = *v.Name
				} else {
					m["name"] = nil
				}
				if v.Cost != nil {
					m["cost"] = *v.Cost
				} else {
					m["cost"] = nil
				}
				list = append(list, m)
			}
			// if off, should be empty per spec? but we always compute; if source==off, spec says all labeled as unlabeled but by_conversation empty? Let's enforce empty for off.
			if source == "off" {
				list = []interface{}{}
			}
			c.JSON(200, gin.H{"byConversation": list, "conversations": list})
			return
		}
		c.JSON(200, gin.H{"byConversation": []interface{}{}})
		return
	}
	// recentRequests path (default)
	rows, err := db.Query(`SELECT ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id,conversation_name FROM requests ORDER BY id DESC LIMIT 20`)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []interface{}{}
	for rows.Next() {
		var ts, provider, model, convID, convName sql.NullString
		var succ, pt, ct, cached, reasoning sql.NullInt64
		var cost sql.NullFloat64
		_ = rows.Scan(&ts, &provider, &model, &succ, &pt, &ct, &cached, &reasoning, &cost, &convID, &convName)
		m := map[string]interface{}{
			"ts":              nil,
			"provider":        nil,
			"model":           nil,
			"success":         nil,
			"prompt_tokens":   nil,
			"completion_tokens": nil,
			"cached_tokens":   nil,
			"reasoning_tokens": nil,
			"cost":            nil,
			"conversation_id": nil,
			"conversation_name": nil,
		}
		if ts.Valid {
			m["ts"] = ts.String
		}
		if provider.Valid {
			m["provider"] = provider.String
		}
		if model.Valid {
			m["model"] = model.String
		}
		if succ.Valid {
			m["success"] = succ.Int64 == 1
		}
		if pt.Valid {
			m["prompt_tokens"] = pt.Int64
		}
		if ct.Valid {
			m["completion_tokens"] = ct.Int64
		}
		if cached.Valid {
			m["cached_tokens"] = cached.Int64
		}
		if reasoning.Valid {
			m["reasoning_tokens"] = reasoning.Int64
		}
		if cost.Valid {
			m["cost"] = cost.Float64
		}
		if convID.Valid {
			m["conversation_id"] = convID.String
		}
		if convName.Valid {
			m["conversation_name"] = convName.String
		}
		out = append(out, m)
	}
	// recentRequests plus alias rows
	c.JSON(200, gin.H{"rows": out, "recentRequests": out, "recent_request_total": len(out)})
}

func effectiveConversationID(convID, convName sql.NullString, provider, model, ts sql.NullString, source string) (string, string) {
	if source == "off" {
		return "unlabeled", ""
	}
	if convID.Valid && convID.String != "" && convID.String != "unlabeled" {
		name := ""
		if convName.Valid {
			name = convName.String
		}
		return convID.String, name
	}
	if source == "proxy" {
		return "unlabeled", ""
	}
	// sessionScan attempt virtual
	sessions := scan.Scan()
	// need ts parse for window
	if !ts.Valid {
		return "unlabeled", ""
	}
	entryTs, err := time.Parse(time.RFC3339, ts.String)
	if err != nil {
		return "unlabeled", ""
	}
	nowMs := time.Now().UnixMilli()
	entryMs := entryTs.UnixMilli()
	modelStr := ""
	if model.Valid {
		modelStr = model.String
		if idx := strings.LastIndex(modelStr, "/"); idx >= 0 {
			modelStr = modelStr[idx+1:]
		}
	}
	bestID, bestTitle, bestScore := "", "", int64(1<<62)
	for _, sess := range sessions {
		sessModel := ""
		if sess.Model != nil {
			sessModel = *sess.Model
			if idx := strings.LastIndex(sessModel, "/"); idx >= 0 {
				sessModel = sessModel[idx+1:]
			}
		}
		if modelStr != "" && sessModel != "" && modelStr != sessModel {
			continue
		}
		if modelStr == "" {
			continue
		}
		if sess.LastActiveAt == nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, *sess.LastActiveAt)
		if err != nil {
			continue
		}
		sessMs := t.UnixMilli()
		diff := entryMs - sessMs
		if diff < 0 {
			diff = -diff
		}
		if diff > 2000 {
			continue
		}
		if nowMs-sessMs > 5*60*1000 || sessMs > nowMs+2000 {
			continue
		}
		score := diff
		if score < bestScore {
			bestScore = score
			bestID = sess.ID
			bestTitle = sess.Title
		}
	}
	if bestID != "" {
		return bestID, bestTitle
	}
	return "unlabeled", ""
}
