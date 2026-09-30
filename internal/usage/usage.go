package usage

import (
	"encoding/json"
	"math"
	"strings"
)

type UsageSummary struct {
	PromptTokens          uint64 `json:"prompt_tokens"`
	CompletionTokens      uint64 `json:"completion_tokens"`
	CachedTokens          uint64 `json:"cached_tokens"`
	ReasoningTokens       uint64 `json:"reasoning_tokens"`
	PromptTokensKnown     bool   `json:"-"`
	CompletionTokensKnown bool   `json:"-"`
	CachedTokensKnown     bool   `json:"-"`
	ReasoningTokensKnown  bool   `json:"-"`
}

func ExtractUsage(v map[string]interface{}) *UsageSummary {
	raw, ok := v["usage"].(map[string]interface{})
	if !ok {
		return nil
	}
	prompt, promptKnown := firstToken(raw, "input_tokens", "prompt_tokens")
	completion, completionKnown := firstToken(raw, "output_tokens", "completion_tokens")
	cached, cachedKnown := firstToken(raw, "cache_read_input_tokens")
	if !cachedKnown {
		for _, key := range []string{"prompt_tokens_details", "input_tokens_details"} {
			if details, ok := raw[key].(map[string]interface{}); ok {
				cached, cachedKnown = firstToken(details, "cached_tokens")
				if cachedKnown {
					break
				}
			}
		}
	}
	if !cachedKnown {
		cached, cachedKnown = firstToken(raw, "cached_tokens", "prompt_cache_hit_tokens")
	}
	var reasoning uint64
	reasoningKnown := false
	for _, key := range []string{"completion_tokens_details", "output_tokens_details"} {
		if details, ok := raw[key].(map[string]interface{}); ok {
			reasoning, reasoningKnown = firstToken(details, "reasoning_tokens")
			if reasoningKnown {
				break
			}
		}
	}
	return &UsageSummary{
		PromptTokens: prompt, CompletionTokens: completion, CachedTokens: cached, ReasoningTokens: reasoning,
		PromptTokensKnown: promptKnown, CompletionTokensKnown: completionKnown,
		CachedTokensKnown: cachedKnown, ReasoningTokensKnown: reasoningKnown,
	}
}

func firstToken(source map[string]interface{}, keys ...string) (uint64, bool) {
	for _, key := range keys {
		switch n := source[key].(type) {
		case float64:
			if n >= 0 && n < float64(uint64(1)<<63) && math.Trunc(n) == n {
				return uint64(n), true
			}
		case int:
			if n >= 0 {
				return uint64(n), true
			}
		case int64:
			if n >= 0 {
				return uint64(n), true
			}
		case uint64:
			if n <= math.MaxInt64 {
				return n, true
			}
		case json.Number:
			if n, err := n.Int64(); err == nil && n >= 0 {
				return uint64(n), true
			}
		}
	}
	return 0, false
}

// NullableTokens keeps missing usage distinct from an explicitly reported zero
// at storage and JSON boundaries. A nil summary contains no token facts.
func (s *UsageSummary) NullableTokens() (prompt, completion, cached, reasoning *int64) {
	if s == nil {
		return
	}
	knownValue := func(value uint64, known bool) *int64 {
		if !known {
			return nil
		}
		n := int64(value)
		return &n
	}
	return knownValue(s.PromptTokens, s.PromptTokensKnown), knownValue(s.CompletionTokens, s.CompletionTokensKnown),
		knownValue(s.CachedTokens, s.CachedTokensKnown), knownValue(s.ReasoningTokens, s.ReasoningTokensKnown)
}

func frameEnd(buf []byte) (int, int) {
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

type SseUsageParser struct {
	buffer              []byte
	summary             *UsageSummary
	anthropicInput      *uint64
	anthropicCached     *uint64
	anthropicCompletion *uint64
}

func NewSseUsageParser() *SseUsageParser { return &SseUsageParser{} }

func (p *SseUsageParser) Push(chunk []byte) {
	p.buffer = append(p.buffer, chunk...)
	for {
		end, sep := frameEnd(p.buffer)
		if end < 0 {
			break
		}
		frame := make([]byte, end)
		copy(frame, p.buffer[:end])
		newBuf := make([]byte, len(p.buffer)-end-sep)
		copy(newBuf, p.buffer[end+sep:])
		p.buffer = newBuf
		p.handleFrame(frame)
	}
}

func (p *SseUsageParser) handleFrame(frame []byte) {
	lines := strings.Split(string(frame), "\n")
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		var v map[string]interface{}
		if err := json.Unmarshal([]byte(data), &v); err != nil {
			continue
		}
		if typ, ok := v["type"].(string); ok {
			switch typ {
			case "message_start":
				if msg, ok := v["message"].(map[string]interface{}); ok {
					if raw, ok := msg["usage"].(map[string]interface{}); ok {
						p.captureAnthropicUsage(raw)
					}
				}
			case "message_delta":
				if raw, ok := v["usage"].(map[string]interface{}); ok {
					p.captureAnthropicUsage(raw)
				}
			default:
				usageSource := v
				if _, ok := v["usage"]; !ok {
					if resp, ok := v["response"].(map[string]interface{}); ok {
						if _, ok := resp["usage"]; ok {
							usageSource = resp
						} else {
							continue
						}
					} else {
						continue
					}
				}
				if s := ExtractUsage(usageSource); s != nil {
					p.summary = s
				}
			}
			continue
		}
		if s := ExtractUsage(v); s != nil {
			p.summary = s
		}
	}
}

func (p *SseUsageParser) captureAnthropicUsage(raw map[string]interface{}) {
	if u, ok := firstToken(raw, "input_tokens"); ok {
		p.anthropicInput = &u
	}
	if u, ok := firstToken(raw, "output_tokens"); ok {
		p.anthropicCompletion = &u
	}
	if u, ok := firstToken(raw, "cache_read_input_tokens"); ok {
		p.anthropicCached = &u
	}
}

func (p *SseUsageParser) Finish() *UsageSummary {
	if p.summary != nil {
		return p.summary
	}
	if p.anthropicInput == nil && p.anthropicCompletion == nil && p.anthropicCached == nil {
		return nil
	}
	result := &UsageSummary{}
	if p.anthropicInput != nil {
		result.PromptTokens = *p.anthropicInput
		result.PromptTokensKnown = true
	}
	if p.anthropicCompletion != nil {
		result.CompletionTokens = *p.anthropicCompletion
		result.CompletionTokensKnown = true
	}
	if p.anthropicCached != nil {
		result.CachedTokens = *p.anthropicCached
		result.CachedTokensKnown = true
	}
	return result
}
