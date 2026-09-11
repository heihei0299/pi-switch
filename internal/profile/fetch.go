package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/heihei0299/pi-switch/internal/catalog"
	"github.com/heihei0299/pi-switch/internal/config"
)

// EnrichCounts reports what EnrichModelsWithCatalog did, so each caller can
// surface the same numbers the WebUI shows.
type EnrichCounts struct {
	Enriched int
	Skipped  int
	Failed   int
	Warning  string
}

// FetchUpstreamIDs tries baseURL/models then baseURL/v1/models with optional
// bearer key and headers. Returns nil ids + lastErr when all attempts fail.
func FetchUpstreamIDs(baseURL, apiKey string, headers map[string]string) ([]string, string) {
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

// FetchUpstreamModelIDs lists the models a profile's primary upstream reports.
// It is the read-only half of POST /api/profiles/:name/fetch-models (the handler
// additionally enriches and persists for channel-directed fetches) and the whole
// of `pi-switch provider fetch-models`. Returns a human-readable reason instead
// of an error so callers can surface it as their own kind of failure.
func FetchUpstreamModelIDs(prof config.ProviderProfile) ([]string, string) {
	return FetchUpstreamIDs(prof.PrimaryBaseURL(), prof.PrimaryAPIKey(), nil)
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

// EnrichModelsWithCatalog fills model seeds from the models.dev snapshot for the
// profile's provider. It returns counters and a warning instead of an error so
// callers can keep serving the un-enriched list.
func EnrichModelsWithCatalog(models []map[string]interface{}, prof config.ProviderProfile) (enriched, skipped, failed int, warning string) {
	providerKey := prof.ModelsDevProviderKey()
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

// FetchChannelModels pulls the model list a channel's own credentials report,
// enriches it, merges only the new ids into that channel's pool and persists.
// It is the single implementation behind
// POST /api/profiles/:name/fetch-models?channel= and
// `pi-switch provider fetch-models --channel`, so the CLI cannot drift from the
// handler's channel-directed semantics. Existing entries are never modified and
// other channels are never touched.
func FetchChannelModels(name, channel string) ([]string, EnrichCounts, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, EnrichCounts{}, err
	}
	prof, ok := cfg.Profiles[name]
	if !ok {
		return nil, EnrichCounts{}, profileErr(ErrProfileNotFound, "profile %q not found", name)
	}
	if channel == "" {
		return nil, EnrichCounts{}, errors.New("channel is required")
	}
	idx := EnsureMutationChannel(&prof, channel)
	if idx < 0 {
		return nil, EnrichCounts{}, profileErr(ErrUnknownChannel, "unknown channel %q", channel)
	}
	u := prof.Upstreams[idx]
	ids, lastErr := FetchUpstreamIDs(u.BaseURL, u.APIKey, u.Headers)
	if ids == nil {
		return nil, EnrichCounts{}, profileErr(ErrUpstreamFetchFailed, "%s", lastErr)
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
	enriched, skipped, failed, warning := EnrichModelsWithCatalog(seeds, prof)
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
	if err := persist(cfg, "failed to save config: "); err != nil {
		return nil, EnrichCounts{}, err
	}
	return ids, EnrichCounts{Enriched: enriched, Skipped: skipped, Failed: failed, Warning: warning}, nil
}
