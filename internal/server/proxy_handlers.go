package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/conversation"
	"github.com/heihei0299/pi-switch/internal/limit"
	"github.com/heihei0299/pi-switch/internal/proxy"
	"github.com/heihei0299/pi-switch/internal/store"
	"github.com/heihei0299/pi-switch/internal/translator"
	"github.com/heihei0299/pi-switch/internal/usage"
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
func resolveRoute(cfg config.PiSwitchConfig, requested string) ([]string, string, string) {
	// Bare model ids are resolved across all exposed channels. The caller
	// rejects slash-containing ids before this function is reached.
	matches := []struct {
		supplier string
		channel  string
	}{}
	for name, prof := range cfg.Profiles {
		for i := range prof.Upstreams {
			channel := prof.Upstreams[i]
			for _, eid := range channel.ExposedModels {
				if eid == requested {
					matches = append(matches, struct {
						supplier string
						channel  string
					}{name, prof.ChannelName(i)})
					break
				}
			}
		}
	}
	if len(matches) == 0 {
		return nil, requested, ""
	}
	if len(matches) == 1 {
		return []string{matches[0].supplier}, requested, matches[0].channel
	}
	// 多处暴露 => ambiguous，需显式消解
	return nil, requested, "ambiguous"
}

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

func findModelEntry(prof config.ProviderProfile, realModel string) *config.ModelEntry {
	for i := range prof.Upstreams {
		for j := range prof.Upstreams[i].Models {
			if prof.Upstreams[i].Models[j].ID == realModel {
				return &prof.Upstreams[i].Models[j]
			}
		}
	}
	return nil
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

func estimateEffectiveLen(body map[string]interface{}, rawLen int) int {
	// For reasoning.encrypted_content base64 dense (≈2.5 chars/token vs 3 for normal text),
	// effectiveLen = rawLen + 0.2*encryptedLen so that est = effectiveLen/3 = (raw-encrypted)/3 + encrypted/2.5
	var encryptedLen int
	var walk func(v interface{})
	walk = func(v interface{}) {
		switch vv := v.(type) {
		case map[string]interface{}:
			for k, val := range vv {
				if k == "encrypted_content" {
					if s, ok := val.(string); ok {
						encryptedLen += len(s)
					}
				}
				walk(val)
			}
		case []interface{}:
			for _, e := range vv {
				walk(e)
			}
		}
	}
	walk(body)
	if encryptedLen > 0 {
		extra := int(math.Ceil(float64(encryptedLen) * 0.2))
		return rawLen + extra
	}
	return rawLen
}

func clampBody(body map[string]interface{}, modelEntry *config.ModelEntry, rawLen int) {
	if modelEntry == nil || modelEntry.ContextWindow == 0 || modelEntry.MaxTokens == 0 {
		return
	}
	effectiveLen := estimateEffectiveLen(body, rawLen)
	for _, key := range []string{"max_tokens", "max_output_tokens", "max_completion_tokens"} {
		if v, ok := body[key]; ok {
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
			clamped := limit.ClampMaxTokens(modelEntry.ContextWindow, modelEntry.MaxTokens, effectiveLen, &reqCopy)
			if clamped != req {
				body[key] = float64(clamped)
			}
		}
	}
}

func findFrameEnd(buf []byte) (int, int) {
	for i := 0; i+3 < len(buf); i++ {
		if buf[i] == '\r' && buf[i+1] == '\n' && buf[i+2] == '\r' && buf[i+3] == '\n' {
			return i, 4
		}
	}
	for i := 0; i+1 < len(buf); i++ {
		if buf[i] == '\n' && buf[i+1] == '\n' {
			return i, 2
		}
	}
	return -1, 0
}

func extractData(frame []byte) string {
	lines := strings.Split(string(frame), "\n")
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "data:") {
			d := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if d != "" {
				return d
			}
		}
	}
	return ""
}

func consumeSSEFrames(buf *bytes.Buffer, chunk []byte, onData func(string)) {
	buf.Write(chunk)
	for {
		raw := buf.Bytes()
		end, sep := findFrameEnd(raw)
		if end < 0 {
			return
		}
		frame := append([]byte(nil), raw[:end]...)
		buf.Next(end + sep)
		onData(extractData(frame))
	}
}

func isSSEFailure(data string, format translator.Format) bool {
	var event map[string]interface{}
	if json.Unmarshal([]byte(data), &event) != nil {
		return false
	}
	typ, _ := event["type"].(string)
	switch format {
	case translator.FormatOpenAIResponses:
		return typ == "response.failed"
	case translator.FormatAnthropic:
		return typ == "error"
	default:
		_, ok := event["error"]
		return ok
	}
}

func isSSETerminal(data string, format translator.Format) bool {
	if format == translator.FormatOpenAIChat {
		return data == "[DONE]"
	}
	var event map[string]interface{}
	if json.Unmarshal([]byte(data), &event) != nil {
		return false
	}
	typ, _ := event["type"].(string)
	switch format {
	case translator.FormatOpenAIResponses:
		return typ == "response.completed" || typ == "response.incomplete" || typ == "response.failed"
	case translator.FormatAnthropic:
		return typ == "message_stop"
	default:
		return false
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
	candidates, realModel, pinnedChannel := resolveRoute(cfg, requestedModel)
	if pinnedChannel == "ambiguous" {
		c.JSON(502, inferenceError(fmt.Sprintf("Ambiguous model '%s': exposed in multiple channels", requestedModel), "ambiguous"))
		return
	}
	if len(candidates) == 0 {
		c.JSON(502, inferenceError(fmt.Sprintf("No upstream exposes model '%s'", requestedModel), "no_route"))
		return
	}
	convID, convName := conversationIDFrom(c.Request.Header, body, cfg.Settings.ConversationSource)
	proto := incomingProtocol(c.Request.URL.Path)
	isStream := false
	if v, ok := body["stream"].(bool); ok && v {
		isStream = true
	}
	if isStream {
		handleStream(c, cfg, candidates, body, realModel, pinnedChannel, convID, convName, proto, rawLen, start)
		return
	}
	// Transitional passthrough: single candidate, no retry, no cooling.
	// retained for per-conversation breaker, not used in transitional passthrough
	name := candidates[0]
	prof, ok := cfg.Profiles[name]
	if !ok {
		c.JSON(502, inferenceError(fmt.Sprintf("No upstream exposes model '%s'", requestedModel), "no_route"))
		return
	}
	if pinnedChannel != "" {
		found := false
		for i, ups := range prof.ResolvedUpstreams() {
			chName := ""
			if ups.Name != nil {
				chName = *ups.Name
			}
			if chName == pinnedChannel {
				prof = narrowToChannel(prof, i)
				found = true
				break
			}
		}
		if !found {
			c.JSON(502, inferenceError(fmt.Sprintf("No upstream exposes model '%s'", requestedModel), "no_route"))
			return
		}
	}
	if pinnedChannel == "" {
		matched := -1
		matchCount := 0
		for i, ups := range prof.ResolvedUpstreams() {
			for _, eid := range ups.ExposedModels {
				if eid == realModel {
					if matched == -1 {
						matched = i
					}
					matchCount++
					break
				}
			}
		}
		if matchCount > 1 {
			c.JSON(502, inferenceError(fmt.Sprintf("Ambiguous model '%s' in supplier '%s': exposed in %d channels, use '%s/<channel>/%s'", requestedModel, name, matchCount, name, realModel), "ambiguous"))
			return
		}
		if matched != -1 {
			prof = narrowToChannel(prof, matched)
		}
	}
	upstream := selectedOutboundUpstream(prof)
	base := upstream.BaseURL
	if base == "" {
		c.JSON(502, inferenceError("missing baseUrl", "no_route"))
		return
	}
	modelEntry := findModelEntry(prof, realModel)
	bcopy := cloneMap(body)
	bcopy["model"] = realModel
	clampBody(bcopy, modelEntry, rawLen)
	plan, planErr := translator.PlanRequest(proto, prof.API, prof.ResponsesMode)
	if planErr != nil {
		c.JSON(502, inferenceError(fmt.Sprintf("profile %s: %s", name, planErr.Error()), "no_route"))
		return
	}
	convBody, convErr := plan.TransformRequest(realModel, bcopy)
	if convErr != nil {
		c.JSON(502, inferenceError(convErr.Error(), "no_route"))
		return
	}
	clampBody(convBody, modelEntry, rawLen)
	upstreamBody := convBody
	upstreamPath := plan.UpstreamPath
	needRespConvert := plan.NeedsConvert()
	bbytes, _ := json.Marshal(upstreamBody)
	outboundPlan := OutboundRequestPlan{
		Upstream:         upstream,
		Path:             upstreamPath,
		ProfileHeaders:   prof.Headers,
		IncomingHeaders:  c.Request.Header,
		Body:             bbytes,
		ContentType:      "application/json",
		ProfileUserAgent: prof.UserAgent,
		GlobalUserAgent:  cfg.Settings.Proxy.UserAgent,
		Timeout:          30 * time.Second,
	}
	outbound, err := BuildOutboundRequest(outboundPlan)
	if err != nil {
		c.JSON(502, inferenceError(err.Error(), "upstream_error"))
		return
	}
	resp, err := outbound.Client.Do(outbound.Request)
	if err != nil {
		logRequest(name, realModel, false, 0, 0, 0, 0, nil, convID, convName, time.Since(start).Milliseconds(), 502, err.Error(), outbound.Metadata.URL)
		c.JSON(502, inferenceError(err.Error(), "upstream_error"))
		return
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		bodyStr := string(respBody)
		shouldRetry := resp.StatusCode == 400 && strings.Contains(bodyStr, "invalid_request_error")
		if shouldRetry {
			// Retry once with max_output_tokens=16 (and other max keys) to recover from context overflow.
			bcopy2 := cloneMap(bcopy)
			for _, k := range []string{"max_tokens", "max_output_tokens", "max_completion_tokens"} {
				bcopy2[k] = float64(16)
			}
			convBody2, convErr2 := plan.TransformRequest(realModel, bcopy2)
			if convErr2 == nil {
				for _, k := range []string{"max_tokens", "max_output_tokens", "max_completion_tokens"} {
					if _, ok := convBody2[k]; ok {
						convBody2[k] = float64(16)
					}
				}
				// Also ensure max_output_tokens is present for Responses
				if _, ok := convBody2["max_output_tokens"]; !ok {
					convBody2["max_output_tokens"] = float64(16)
				}
				clampBody(convBody2, modelEntry, rawLen)
				bbytes2, _ := json.Marshal(convBody2)
				retryPlan := outboundPlan
				retryPlan.Body = bbytes2
				outbound2, err2 := BuildOutboundRequest(retryPlan)
				if err2 == nil {
					resp2, err2 := outbound2.Client.Do(outbound2.Request)
					if err2 == nil {
						respBody2, _ := io.ReadAll(resp2.Body)
						resp2.Body.Close()
						if resp2.StatusCode < 400 {
							// Success on retry: handle as normal success
							finalBody2 := respBody2
							finalHeaders2 := resp2.Header
							if needRespConvert {
								var upstreamObj map[string]interface{}
								if err := json.Unmarshal(respBody2, &upstreamObj); err == nil {
									if conv, err := plan.TransformResponse(upstreamObj, realModel); err == nil {
										b, _ := json.Marshal(conv)
										finalBody2 = b
										finalHeaders2 = http.Header{}
										finalHeaders2.Set("Content-Type", "application/json")
									}
								}
							}
							var respObj map[string]interface{}
							_ = json.Unmarshal(finalBody2, &respObj)
							usagePrompt, usageCompletion, usageCached, usageReasoning := extractUsage(respObj)
							if usagePrompt == 0 && usageCompletion == 0 {
								if s := usage.ExtractUsage(respObj); s != nil {
									usagePrompt = int(s.PromptTokens)
									usageCompletion = int(s.CompletionTokens)
									usageCached = int(s.CachedTokens)
									usageReasoning = int(s.ReasoningTokens)
								}
							}
							cost := proxy.CalcCost(modelEntry, usagePrompt, usageCompletion, usageCached)
							latMs := time.Since(start).Milliseconds()
							logRequest(name, realModel, true, usagePrompt, usageCompletion, usageCached, usageReasoning, cost, convID, convName, latMs, resp2.StatusCode, "", outbound2.Metadata.URL)
							for k, vv := range finalHeaders2 {
								for _, v := range vv {
									if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Connection") {
										continue
									}
									c.Header(k, v)
								}
							}
							ct := finalHeaders2.Get("Content-Type")
							if ct == "" {
								ct = "application/json"
							}
							c.Data(resp2.StatusCode, ct, finalBody2)
							return
						}
						// Retry also failed: fall through to original 400 handling but log retry body
						_ = respBody2
					}
				}
			}
		}
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
		logRequest(name, realModel, false, 0, 0, 0, 0, nil, convID, convName, time.Since(start).Milliseconds(), resp.StatusCode, string(respBody), outbound.Metadata.URL)
		return
	}
	finalBody := respBody
	finalHeaders := resp.Header
	if needRespConvert {
		var upstreamObj map[string]interface{}
		if err := json.Unmarshal(respBody, &upstreamObj); err == nil {
			if conv, err := plan.TransformResponse(upstreamObj, realModel); err == nil {
				b, _ := json.Marshal(conv)
				finalBody = b
				finalHeaders = http.Header{}
				finalHeaders.Set("Content-Type", "application/json")
			}
		}
	}
	var respObj map[string]interface{}
	_ = json.Unmarshal(finalBody, &respObj)
	usagePrompt, usageCompletion, usageCached, usageReasoning := extractUsage(respObj)
	if usagePrompt == 0 && usageCompletion == 0 {
		if s := usage.ExtractUsage(respObj); s != nil {
			usagePrompt = int(s.PromptTokens)
			usageCompletion = int(s.CompletionTokens)
			usageCached = int(s.CachedTokens)
			usageReasoning = int(s.ReasoningTokens)
		}
	}
	cost := proxy.CalcCost(modelEntry, usagePrompt, usageCompletion, usageCached)
	latMs := time.Since(start).Milliseconds()
	logRequest(name, realModel, true, usagePrompt, usageCompletion, usageCached, usageReasoning, cost, convID, convName, latMs, resp.StatusCode, "", outbound.Metadata.URL)
	for k, vv := range finalHeaders {
		for _, v := range vv {
			if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Connection") {
				continue
			}
			c.Header(k, v)
		}
	}
	ct := finalHeaders.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}
	c.Data(resp.StatusCode, ct, finalBody)
}

func handleStream(c *gin.Context, cfg config.PiSwitchConfig, candidates []string, body map[string]interface{}, realModel, pinnedChannel, convID, convName, proto string, rawLen int, start time.Time) {
	// Transitional passthrough: single candidate, no retry, no cooling.
	if len(candidates) == 0 {
		c.JSON(502, inferenceError("No upstream exposes model", "no_route"))
		return
	}
	name := candidates[0]
	prof, ok := cfg.Profiles[name]
	if !ok {
		c.JSON(502, inferenceError("No upstream exposes model", "no_route"))
		return
	}
	if pinnedChannel != "" {
		found := false
		for i, ups := range prof.ResolvedUpstreams() {
			chName := ""
			if ups.Name != nil {
				chName = *ups.Name
			}
			if chName == pinnedChannel {
				prof = narrowToChannel(prof, i)
				found = true
				break
			}
		}
		if !found {
			c.JSON(502, inferenceError("No upstream exposes model", "no_route"))
			return
		}
	}
	if pinnedChannel == "" {
		matched := -1
		matchCount := 0
		for i, ups := range prof.ResolvedUpstreams() {
			for _, eid := range ups.ExposedModels {
				if eid == realModel {
					if matched == -1 {
						matched = i
					}
					matchCount++
					break
				}
			}
		}
		if matchCount > 1 {
			c.JSON(502, inferenceError(fmt.Sprintf("Ambiguous model '%s/%s' in supplier '%s': exposed in %d channels, use '%s/<channel>/%s'", name, realModel, name, matchCount, name, realModel), "ambiguous"))
			return
		}
		if matched != -1 {
			prof = narrowToChannel(prof, matched)
		}
	}
	upstream := selectedOutboundUpstream(prof)
	base := upstream.BaseURL
	if base == "" {
		c.JSON(502, inferenceError("missing baseUrl", "no_route"))
		return
	}
	modelEntry := findModelEntry(prof, realModel)
	bcopy := cloneMap(body)
	bcopy["model"] = realModel
	bcopy["stream"] = true
	clampBody(bcopy, modelEntry, rawLen)
	plan, planErr := translator.PlanRequest(proto, prof.API, prof.ResponsesMode)
	if planErr != nil {
		c.JSON(502, inferenceError(fmt.Sprintf("profile %s: %s", name, planErr.Error()), "no_route"))
		return
	}
	convBody, convErr := plan.TransformRequest(realModel, bcopy)
	if convErr != nil {
		c.JSON(502, inferenceError(convErr.Error(), "no_route"))
		return
	}
	convBody["stream"] = true
	clampBody(convBody, modelEntry, rawLen)
	upstreamBody := convBody
	upstreamPath := plan.UpstreamPath
	bbytes, _ := json.Marshal(upstreamBody)
	outbound, err := BuildOutboundRequest(OutboundRequestPlan{
		Upstream:         upstream,
		Path:             upstreamPath,
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
		logRequest(name, realModel, false, 0, 0, 0, 0, nil, convID, convName, time.Since(start).Milliseconds(), 502, err.Error(), outbound.Metadata.URL)
		c.JSON(502, inferenceError(err.Error(), "upstream_error"))
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
		logRequest(name, realModel, false, 0, 0, 0, 0, nil, convID, convName, time.Since(start).Milliseconds(), resp.StatusCode, string(bodyBytes), outbound.Metadata.URL)
		return
	}
	if conv := plan.StreamConverter(realModel); conv != nil {
		streamConvert(c, resp, conv, plan.From == translator.FormatOpenAIResponses, name, realModel, modelEntry, convID, convName, start)
		return
	}
	streamPassthrough(c, resp, plan.To, name, realModel, modelEntry, convID, convName, start)
}

func streamPassthrough(c *gin.Context, resp *http.Response, upstreamFormat translator.Format, provider, realModel string, modelEntry *config.ModelEntry, convID, convName string, start time.Time) {
	defer resp.Body.Close()
	parser := usage.NewSseUsageParser()
	for k, vv := range resp.Header {
		for _, v := range vv {
			if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Connection") {
				continue
			}
			c.Header(k, v)
		}
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(resp.StatusCode)
	flusher, _ := c.Writer.(http.Flusher)
	buf := make([]byte, 4096)
	var totalBytes bytes.Buffer
	var frames bytes.Buffer
	terminalSeen := false
	logicalFailure := false
	var readErr error
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			parser.Push(chunk)
			totalBytes.Write(chunk)
			consumeSSEFrames(&frames, chunk, func(data string) {
				if isSSETerminal(data, upstreamFormat) {
					terminalSeen = true
				}
				if isSSEFailure(data, upstreamFormat) {
					logicalFailure = true
				}
			})
			_, _ = c.Writer.Write(chunk)
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			readErr = err
			break
		}
	}
	usageSum := parser.Finish()
	var prompt, completion, cached, reasoning int
	var cost *float64
	if usageSum != nil {
		prompt = int(usageSum.PromptTokens)
		completion = int(usageSum.CompletionTokens)
		cached = int(usageSum.CachedTokens)
		reasoning = int(usageSum.ReasoningTokens)
		cost = proxy.CalcCost(modelEntry, prompt, completion, cached)
	} else {
		var respObj map[string]interface{}
		_ = json.Unmarshal(totalBytes.Bytes(), &respObj)
		prompt, completion, cached, reasoning = extractUsage(respObj)
		cost = proxy.CalcCost(modelEntry, prompt, completion, cached)
	}
	streamFailed := logicalFailure || (readErr != nil && !errors.Is(readErr, io.EOF))
	errorMessage := ""
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		errorMessage = readErr.Error()
	} else if logicalFailure {
		errorMessage = "upstream stream failed"
	} else if !terminalSeen {
		streamFailed = true
		errorMessage = "upstream stream ended without terminal event"
	}
	statusCode := resp.StatusCode
	if streamFailed {
		statusCode = http.StatusBadGateway
	}
	latMs := time.Since(start).Milliseconds()
	logRequest(provider, realModel, !streamFailed, prompt, completion, cached, reasoning, cost, convID, convName, latMs, statusCode, errorMessage, requestURLOf(resp))
}

// streamConvert relays an upstream SSE stream through a registry converter.
// responsesStyle selects Responses event lines ("event: <type>") versus Chat
// bare data lines (terminated with "data: [DONE]").
func streamConvert(c *gin.Context, resp *http.Response, conv translator.StreamEventConverter, responsesStyle bool, provider, realModel string, modelEntry *config.ModelEntry, convID, convName string, start time.Time) {
	defer resp.Body.Close()
	parser := usage.NewSseUsageParser()
	for k, vv := range resp.Header {
		for _, v := range vv {
			if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Connection") {
				continue
			}
			c.Header(k, v)
		}
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(resp.StatusCode)
	flusher, _ := c.Writer.(http.Flusher)
	streamFailed := false
	var streamErr error
	terminalSeen := false
	upstreamFormat := translator.FormatOpenAIResponses
	if responsesStyle {
		upstreamFormat = translator.FormatOpenAIChat
	}
	emit := func(ev map[string]interface{}) {
		if _, ok := ev["error"]; ok {
			streamFailed = true
		}
		if typ, ok := ev["type"].(string); ok && typ == "response.failed" {
			streamFailed = true
		}
		b, _ := json.Marshal(ev)
		var line string
		if responsesStyle {
			typ, _ := ev["type"].(string)
			line = fmt.Sprintf("event: %s\ndata: %s\n\n", typ, string(b))
		} else {
			line = fmt.Sprintf("data: %s\n\n", string(b))
		}
		_, _ = c.Writer.Write([]byte(line))
		if flusher != nil {
			flusher.Flush()
		}
	}
	var buf bytes.Buffer
	tmp := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			chunk := tmp[:n]
			parser.Push(chunk)
			consumeSSEFrames(&buf, chunk, func(data string) {
				if data == "" {
					return
				}
				if isSSETerminal(data, upstreamFormat) {
					terminalSeen = true
				}
				if data == "[DONE]" {
					return
				}
				var v map[string]interface{}
				if err := json.Unmarshal([]byte(data), &v); err != nil {
					return
				}
				events, err := conv.PushEvent(v)
				if err != nil {
					streamFailed = true
					streamErr = err
					return
				}
				for _, ev := range events {
					emit(ev)
				}
			})
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				streamFailed = true
				streamErr = err
			}
			break
		}
	}
	if !streamFailed && !terminalSeen {
		streamFailed = true
		streamErr = errors.New("upstream stream ended without terminal event")
	}
	if !streamFailed {
		for _, ev := range conv.Finish() {
			emit(ev)
		}
		if !responsesStyle {
			_, _ = c.Writer.Write([]byte("data: [DONE]\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
	usageSum := parser.Finish()
	var prompt, completion, cached, reasoning int
	var cost *float64
	if usageSum != nil {
		prompt = int(usageSum.PromptTokens)
		completion = int(usageSum.CompletionTokens)
		cached = int(usageSum.CachedTokens)
		reasoning = int(usageSum.ReasoningTokens)
		cost = proxy.CalcCost(modelEntry, prompt, completion, cached)
	} else if m, ok := conv.UsagePayload().(map[string]interface{}); ok {
		if s := usage.ExtractUsage(map[string]interface{}{"usage": m}); s != nil {
			prompt = int(s.PromptTokens)
			completion = int(s.CompletionTokens)
			cached = int(s.CachedTokens)
			reasoning = int(s.ReasoningTokens)
			cost = proxy.CalcCost(modelEntry, prompt, completion, cached)
		}
	}
	statusCode := resp.StatusCode
	errorMessage := ""
	if streamFailed {
		statusCode = http.StatusBadGateway
		if streamErr != nil {
			errorMessage = streamErr.Error()
		} else {
			errorMessage = "upstream stream failed"
		}
	}
	latMs := time.Since(start).Milliseconds()
	logRequest(provider, realModel, !streamFailed, prompt, completion, cached, reasoning, cost, convID, convName, latMs, statusCode, errorMessage, requestURLOf(resp))
}

func cloneMap(m map[string]interface{}) map[string]interface{} {
	b, _ := json.Marshal(m)
	var out map[string]interface{}
	_ = json.Unmarshal(b, &out)
	return out
}

func extractUsage(resp map[string]interface{}) (prompt, completion, cached, reasoning int) {
	summary := usage.ExtractUsage(resp)
	if summary == nil {
		return 0, 0, 0, 0
	}
	return int(summary.PromptTokens), int(summary.CompletionTokens), int(summary.CachedTokens), int(summary.ReasoningTokens)
}

func logRequest(provider, model string, success bool, prompt, completion, cached, reasoning int, cost *float64, convID, convName string, latency int64, status int, errMsg, upstreamURL string) {
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	succ := 0
	if success {
		succ = 1
	}
	var costVal interface{}
	if cost != nil {
		costVal = *cost
	}
	entry := legacyLogEntry(ts, provider, model, success, prompt, completion, cached, reasoning, cost, convID, convName, status, errMsg, upstreamURL)
	if db, err := store.GetDB(); err != nil {
		log.Printf("request log database: %v", err)
	} else if _, err := db.Exec(`INSERT INTO requests(ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id,conversation_name,latency_ms) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		ts, provider, model, succ, prompt, completion, cached, reasoning, costVal, convID, convName, latency); err != nil {
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
