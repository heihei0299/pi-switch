package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/conversation"
	"github.com/heihei0299/pi-switch/internal/store"
	"github.com/heihei0299/pi-switch/internal/usage"
	"github.com/heihei0299/pi-switch/internal/usecase/inference"
)

func handleModels(c *gin.Context) {
	cfg, ok := loadConfigOrChatError(c)
	if !ok {
		return
	}
	data := []interface{}{}
	seen := map[string]bool{}
	for name, prof := range cfg.Profiles {
		for i := range prof.Upstreams {
			channel := prof.Upstreams[i]
			providerKey := name + "/" + prof.ChannelName(i)
			for _, mid := range channel.ExposedModels {
				key := providerKey + "/" + mid
				if seen[key] {
					continue
				}
				seen[key] = true
				data = append(data, map[string]interface{}{"id": mid, "object": "model", "owned_by": providerKey})
			}
		}
	}
	c.JSON(200, gin.H{"object": "list", "data": data})
}

// --- proxy helpers ---
// pinChannelAttempts keeps only attempts hitting the pinned channel of its
// supplier. Empty pin (legacy/bare ids) keeps everything: failover, weight
// order and retry rounds are untouched.
func pinChannelAttempts(cfg *config.PiSwitchConfig, attempts []attempt, candidates []string, pinned string) []attempt {
	if pinned == "" || cfg == nil {
		return attempts
	}
	supplier := ""
	if len(candidates) > 0 {
		supplier = candidates[0]
	}
	prof, ok := cfg.Profiles[supplier]
	if !ok {
		return attempts
	}
	ups := prof.ResolvedUpstreams()
	keep := map[int]bool{}
	for idx, u := range ups {
		name := ""
		if u.Name != nil {
			name = *u.Name
		}
		if name == pinned {
			keep[idx] = true
		}
	}
	out := make([]attempt, 0, len(attempts))
	for _, att := range attempts {
		if att.ref.name == supplier && keep[att.ref.upsIdx] {
			out = append(out, att)
		}
	}
	return out
}
func conversationIDFrom(headers http.Header, body map[string]interface{}, source string) (string, string) {
	id := ""
	if v := headers.Get("x-conversation-id"); v != "" {
		id = v
	} else if v := headers.Get("x-opencode-session"); v != "" {
		id = v
	} else if v, ok := body["conversation_id"].(string); ok {
		id = v
	}
	model, _ := body["model"].(string)
	var promptHint *uint64
	if pt, ok := body["prompt_tokens"].(float64); ok && pt >= 0 {
		u := uint64(pt)
		promptHint = &u
	}
	var candidates []conversation.Candidate
	if strings.TrimSpace(id) == "" || strings.TrimSpace(id) == conversation.UnlabeledID {
		candidates = conversationCandidates(sessionScanCandidates(source))
	}
	result := conversation.Match(conversation.Source(source), conversation.MatchInput{
		ExplicitID:   id,
		ExplicitName: conversation.SanitizeDisplayName(headers.Get("x-conversation-name")),
		Model:        model,
		Timestamp:    time.Now(),
		PromptTokens: promptHint,
	}, candidates)
	return result.ID, result.Name
}

func incomingProtocol(path string) string {
	switch path {
	case "/v1/responses":
		return "responses"
	case "/v1/messages":
		return "messages"
	default:
		return "chat"
	}
}

// maxProxyBodyEnv names the environment variable that overrides the proxy body
// cap. It lives here, next to its only consumer, so the cap cannot drift away
// from the code that enforces it.
const maxProxyBodyEnv = "PI_SWITCH_MAX_BODY_BYTES"

// defaultMaxProxyBodyBytes is 32 MiB.
const defaultMaxProxyBodyBytes int64 = 32 << 20

// ProxyBodyLimit reports the body cap the proxy routes enforce, for read-only
// callers such as `pi-switch doctor`.
func ProxyBodyLimit() int64 { return maxProxyBodyBytes() }

// maxProxyBodyBytes returns the request body cap for the proxy routes. An
// unparsable or non-positive value falls back to the default rather than
// disabling the cap, so a typo can never remove the bound.
func maxProxyBodyBytes() int64 {
	if v := strings.TrimSpace(os.Getenv(maxProxyBodyEnv)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return defaultMaxProxyBodyBytes
}

func handleChatCompletions(c *gin.Context) {
	start := time.Now()
	// Cap the body before buffering it: an unbounded read is a memory
	// exhaustion vector once the listener is reachable beyond loopback. A
	// rejected request stops here, before any upstream call, logging or billing.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxProxyBodyBytes())
	limited := c.Request.Body
	raw, readErr := io.ReadAll(limited)
	var tooLarge *http.MaxBytesError
	if errors.As(readErr, &tooLarge) {
		c.JSON(http.StatusRequestEntityTooLarge, inferenceError(fmt.Sprintf("request body exceeds the %d byte limit", tooLarge.Limit), "request_too_large"))
		return
	}
	cfg, ok := loadConfigOrChatError(c)
	if !ok {
		return
	}
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		body = map[string]interface{}{}
	}
	rawLen := len(raw)
	requestedModel, _ := body["model"].(string)
	if requestedModel == "" {
		requestedModel = "gpt-4o-mini"
	}
	if strings.Contains(requestedModel, "/") {
		c.JSON(400, inferenceError("model must not contain \"/\"", "invalid_request_error"))
		return
	}
	proto := incomingProtocol(c.Request.URL.Path)
	isStream := false
	if v, ok := body["stream"].(bool); ok && v {
		isStream = true
	}
	prepared, err := inference.PrepareRequest(cfg, requestedModel, proto, body, rawLen, isStream)
	if err != nil {
		var preparationError *inference.PreparationError
		if errors.As(err, &preparationError) {
			c.JSON(http.StatusBadGateway, inferenceError(preparationError.Message, preparationError.Type))
		} else {
			c.JSON(http.StatusBadGateway, inferenceError("request preparation failed", "no_route"))
		}
		return
	}
	convID, convName := conversationIDFrom(c.Request.Header, body, cfg.Settings.ConversationSource)
	if isStream {
		handleStream(c, cfg, prepared, convID, convName, start)
		return
	}
	name, realModel := prepared.Supplier, prepared.Model
	prof, upstream := prepared.Profile, prepared.Upstream
	plan := prepared.ProtocolPlan
	outboundPlan := OutboundRequestPlan{
		Context:          c.Request.Context(),
		Upstream:         upstream,
		Path:             plan.UpstreamPath,
		ProfileHeaders:   prof.Headers,
		IncomingHeaders:  c.Request.Header,
		ContentType:      "application/json",
		ProfileUserAgent: prof.UserAgent,
		GlobalUserAgent:  cfg.Settings.Proxy.UserAgent,
		Timeout:          30 * time.Second,
	}
	buildOutbound := func(body []byte) (inference.OutboundRequest, error) {
		requestPlan := outboundPlan
		requestPlan.Body = body
		request, err := BuildOutboundRequest(requestPlan)
		if err != nil {
			return inference.OutboundRequest{}, err
		}
		return inference.OutboundRequest{Request: request.Request, Client: request.Client, URL: request.Metadata.URL}, nil
	}
	outcome := inference.ExecuteNonStream(c.Request.Context(), prepared, buildOutbound)
	logOutcome := func() {
		if outcome.ShouldLog {
			logRequest(name, realModel, outcome.Kind == inference.NonStreamOutcomeSuccess, outcome.Usage, outcome.Cost, convID, convName,
				time.Since(start).Milliseconds(), outcome.Status, outcome.Error, outcome.URL)
		}
	}
	switch outcome.Kind {
	case inference.NonStreamOutcomeCanceled:
		logOutcome()
		return
	case inference.NonStreamOutcomeError:
		logOutcome()
		c.JSON(outcome.Status, inferenceError(outcome.Error, outcome.ErrorType))
		return
	case inference.NonStreamOutcomeUpstreamResponse:
		c.Data(outcome.Status, outcome.Headers.Get("Content-Type"), outcome.Body)
		logOutcome()
		return
	case inference.NonStreamOutcomeSuccess:
		logOutcome()
	}
	finalHeaders := outcome.Headers
	if prepared.ProtocolPlan.NeedsConvert() {
		finalHeaders = http.Header{}
		finalHeaders.Set("Content-Type", "application/json")
	}
	for key, values := range finalHeaders {
		for _, value := range values {
			if strings.EqualFold(key, "Content-Length") || strings.EqualFold(key, "Transfer-Encoding") || strings.EqualFold(key, "Connection") {
				continue
			}
			c.Header(key, value)
		}
	}
	contentType := finalHeaders.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(outcome.Status, contentType, outcome.Body)
}

func handleStream(c *gin.Context, cfg config.PiSwitchConfig, prepared inference.PreparedRequest, convID, convName string, start time.Time) {
	// Transitional passthrough: single candidate, no retry, no cooling.
	name, realModel := prepared.Supplier, prepared.Model
	prof, upstream, modelEntry := prepared.Profile, prepared.Upstream, prepared.ModelEntry
	plan := prepared.ProtocolPlan
	bbytes, _ := json.Marshal(prepared.Body)
	outbound, err := BuildOutboundRequest(OutboundRequestPlan{
		Context:          c.Request.Context(),
		Upstream:         upstream,
		Path:             plan.UpstreamPath,
		ProfileHeaders:   prof.Headers,
		IncomingHeaders:  c.Request.Header,
		Body:             bbytes,
		ContentType:      "application/json",
		Accept:           "text/event-stream",
		ProfileUserAgent: prof.UserAgent,
		GlobalUserAgent:  cfg.Settings.Proxy.UserAgent,
		Timeout:          0,
	})
	if err != nil {
		c.JSON(502, inferenceError(err.Error(), "upstream_error"))
		return
	}
	resp, err := outbound.Client.Do(outbound.Request)
	if err != nil {
		status := upstreamErrorStatus(c.Request.Context())
		logRequest(name, realModel, false, nil, nil, convID, convName, time.Since(start).Milliseconds(), status, err.Error(), outbound.Metadata.URL)
		if status != 499 {
			c.JSON(status, inferenceError(err.Error(), "upstream_error"))
		}
		return
	}
	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, streamErrorBodyLimit))
		resp.Body.Close()
		for k, vv := range resp.Header {
			for _, v := range vv {
				c.Header(k, v)
			}
		}
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), bodyBytes)
		logRequest(name, realModel, false, nil, nil, convID, convName, time.Since(start).Milliseconds(), resp.StatusCode, string(bodyBytes), outbound.Metadata.URL)
		return
	}
	c.Status(resp.StatusCode)
	facts := inference.RelayStream(c.Request.Context(), inference.StreamInput{
		UpstreamStatus:   resp.StatusCode,
		UpstreamHeaders:  resp.Header,
		Body:             resp.Body,
		UpstreamFormat:   plan.To,
		DownstreamFormat: plan.From,
		Converter:        plan.StreamConverter(realModel),
		ModelEntry:       modelEntry,
	}, c.Writer)
	logRequest(name, realModel, facts.Success, facts.Usage, facts.Cost, convID, convName,
		time.Since(start).Milliseconds(), facts.Status, facts.Error, requestURLOf(resp))
}

// An aborted downstream request has no upstream HTTP status to return.
func upstreamErrorStatus(ctx context.Context) int {
	if ctx.Err() != nil {
		return 499
	}
	return http.StatusBadGateway
}

func logRequest(provider, model string, success bool, summary *usage.UsageSummary, cost *float64, convID, convName string, latency int64, status int, errMsg, upstreamURL string) {
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	succ := 0
	if success {
		succ = 1
	}
	var costVal interface{}
	if cost != nil {
		costVal = *cost
	}
	prompt, completion, cached, reasoning := summary.NullableTokens()
	entry := legacyLogEntry(ts, provider, model, success, summary, cost, convID, convName, status, errMsg, upstreamURL)
	if db, err := store.GetDB(); err != nil {
		log.Printf("request log database: %v", err)
	} else if _, err := db.Exec(`INSERT INTO requests(ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id,conversation_name,latency_ms,status,error,upstream_url) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,NULLIF(?,''),NULLIF(?,''))`,
		ts, provider, model, succ, prompt, completion, cached, reasoning, costVal, convID, convName, latency, status, errMsg, upstreamURL); err != nil {
		log.Printf("request log insert: %v", err)
	}
	if err := appendLegacyLog(entry); err != nil {
		log.Printf("request log legacy: %v", err)
	}
}

func dump400(model, errMsg string, body []byte) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".pi-switch", "400-dump")
	_ = os.MkdirAll(dir, 0755)
	ts := time.Now().Format("20060102-150405")
	safeModel := strings.ReplaceAll(model, "/", "-")
	if safeModel == "" {
		safeModel = "unknown"
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.json", ts, safeModel))
	// Keep only last 10
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(files) >= 10 {
		// Remove oldest (lexicographically first is oldest due to ts prefix)
		_ = os.Remove(files[0])
	}
	payload := map[string]interface{}{
		"ts":    time.Now().Format(time.RFC3339),
		"model": model,
		"error": errMsg,
		"body":  string(body[:min(len(body), 8192)]),
	}
	b, _ := json.MarshalIndent(payload, "", "  ")
	_ = os.WriteFile(path, b, 0644)
}

func handleDumps(c *gin.Context) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".pi-switch", "400-dump")
	entries, err := os.ReadDir(dir)
	if err != nil {
		c.JSON(200, []interface{}{})
		return
	}
	var out []map[string]interface{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		var m map[string]interface{}
		if err := json.Unmarshal(b, &m); err == nil {
			out = append(out, map[string]interface{}{"file": e.Name(), "data": m})
		} else {
			out = append(out, map[string]interface{}{"file": e.Name()})
		}
	}
	c.JSON(200, out)
}
