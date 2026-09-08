package usage

import (
	"encoding/json"
	"strings"
)

type UsageSummary struct {
	PromptTokens         uint64 `json:"prompt_tokens"`
	CompletionTokens     uint64 `json:"completion_tokens"`
	CachedTokens         uint64 `json:"cached_tokens"`
	ReasoningTokens      uint64 `json:"reasoning_tokens"`
	CachedTokensKnown    bool   `json:"-"`
	ReasoningTokensKnown bool   `json:"-"`
}

func ExtractUsage(v map[string]interface{}) *UsageSummary {
	raw, ok := v["usage"]
	if !ok {
		return nil
	}
	usage, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	firstU64 := func(keys ...string) uint64 {
		for _, k := range keys {
			if val, ok := usage[k]; ok {
				if f, ok := val.(float64); ok {
					return uint64(f)
				}
			}
		}
		return 0
	}
	cached := uint64(0)
	cachedKnown := false
	if v, ok := usage["cache_read_input_tokens"]; ok {
		if f, ok := v.(float64); ok {
			cached = uint64(f)
			cachedKnown = true
		}
	}
	if !cachedKnown {
		for _, key := range []string{"prompt_tokens_details", "input_tokens_details"} {
			if details, ok := usage[key].(map[string]interface{}); ok {
				if v, ok := details["cached_tokens"]; ok {
					if f, ok := v.(float64); ok {
						cached = uint64(f)
						cachedKnown = true
						break
					}
				}
			}
		}
	}
	if !cachedKnown {
		if v, ok := usage["cached_tokens"]; ok {
			if f, ok := v.(float64); ok {
				cached = uint64(f)
				cachedKnown = true
			}
		}
	}
	if !cachedKnown {
		if v, ok := usage["prompt_cache_hit_tokens"]; ok {
			if f, ok := v.(float64); ok {
				cached = uint64(f)
				cachedKnown = true
			}
		}
	}
	reasoning := uint64(0)
	reasoningKnown := false
	if details, ok := usage["completion_tokens_details"].(map[string]interface{}); ok {
		if v, ok := details["reasoning_tokens"]; ok {
			if f, ok := v.(float64); ok {
				reasoning = uint64(f)
				reasoningKnown = true
			}
		}
	}
	if !reasoningKnown {
		if details, ok := usage["output_tokens_details"].(map[string]interface{}); ok {
			if v, ok := details["reasoning_tokens"]; ok {
				if f, ok := v.(float64); ok {
					reasoning = uint64(f)
					reasoningKnown = true
				}
			}
		}
	}
	return &UsageSummary{
		PromptTokens:         firstU64("input_tokens", "prompt_tokens"),
		CompletionTokens:     firstU64("output_tokens", "completion_tokens"),
		CachedTokens:         cached,
		ReasoningTokens:      reasoning,
		CachedTokensKnown:    cachedKnown,
		ReasoningTokensKnown: reasoningKnown,
	}
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
					if usage, ok := msg["usage"].(map[string]interface{}); ok {
						if val, ok := usage["input_tokens"].(float64); ok {
							u := uint64(val)
							p.anthropicInput = &u
						}
						if val, ok := usage["cache_read_input_tokens"]; ok {
							if f, ok := val.(float64); ok {
								u := uint64(f)
								p.anthropicCached = &u
							}
						}
					}
				}
			case "message_delta":
				if usage, ok := v["usage"].(map[string]interface{}); ok {
					if val, ok := usage["output_tokens"].(float64); ok {
						u := uint64(val)
						p.anthropicCompletion = &u
					}
					if val, ok := usage["cache_read_input_tokens"]; ok {
						if f, ok := val.(float64); ok {
							u := uint64(f)
							p.anthropicCached = &u
						}
					}
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

func (p *SseUsageParser) Finish() *UsageSummary {
	if p.summary != nil {
		return p.summary
	}
	if p.anthropicInput != nil && p.anthropicCompletion != nil {
		cached := uint64(0)
		if p.anthropicCached != nil {
			cached = *p.anthropicCached
		}
		return &UsageSummary{
			PromptTokens:      *p.anthropicInput,
			CompletionTokens:  *p.anthropicCompletion,
			CachedTokens:      cached,
			CachedTokensKnown: p.anthropicCached != nil,
			ReasoningTokens:   0,
		}
	}
	return nil
}
