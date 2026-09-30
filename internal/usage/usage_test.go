package usage

import (
	"math"
	"testing"
)

func TestAnthropicCacheTotalsAndResponsesCachedSubset(t *testing.T) {
	for _, tc := range []struct {
		name   string
		raw    map[string]interface{}
		prompt uint64
		known  bool
	}{
		{"anthropic", map[string]interface{}{"input_tokens": 100, "cache_creation_input_tokens": 200, "cache_read_input_tokens": 700}, 1000, true},
		{"missing cache write", map[string]interface{}{"input_tokens": 100, "cache_read_input_tokens": 0}, 100, false},
		{"missing cache read", map[string]interface{}{"input_tokens": 100, "cache_creation_input_tokens": 0}, 100, false},
		{"responses", map[string]interface{}{"input_tokens": 100, "input_tokens_details": map[string]interface{}{"cached_tokens": 70}}, 100, true},
		{"zero", map[string]interface{}{"input_tokens": 0, "cache_creation_input_tokens": 0, "cache_read_input_tokens": 0}, 0, true},
		{"invalid cache write", map[string]interface{}{"input_tokens": 100, "cache_creation_input_tokens": -1, "cache_read_input_tokens": 0}, 100, false},
		{"overflow", map[string]interface{}{"input_tokens": int64(math.MaxInt64), "cache_creation_input_tokens": 0, "cache_read_input_tokens": 1}, math.MaxInt64, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := ExtractUsage(map[string]interface{}{"usage": tc.raw})
			if s.PromptTokens != tc.prompt || s.PromptTokensKnown != tc.known {
				t.Fatalf("summary=%+v, want prompt=%d known=%t", s, tc.prompt, tc.known)
			}
		})
	}
}

func TestAnthropicStreamCacheUsageMergesDeltaAndSplitFrames(t *testing.T) {
	p := NewSseUsageParser()
	p.Push([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":100,\"cache_creation_input_tokens\":0,\"cache_read_input_tokens\":700,\"output_tokens\":0}}}\r\n\r"))
	p.Push([]byte("\ndata: {\"type\":\"message_delta\",\"usage\":{\"cache_creation_input_tokens\":200,\"output_tokens\":50}}\n\n"))
	s := p.Finish()
	if s == nil || s.PromptTokens != 1000 || s.CompletionTokens != 50 || s.CachedTokens != 700 || s.CacheWriteTokens != 200 || !s.CacheWriteTokensKnown {
		t.Fatalf("stream usage=%+v", s)
	}
}

func TestExtractUsagePreservesCachedAndReasoningDetails(t *testing.T) {
	summary := ExtractUsage(map[string]interface{}{
		"usage": map[string]interface{}{
			"input_tokens":          100.0,
			"output_tokens":         50.0,
			"input_tokens_details":  map[string]interface{}{"cached_tokens": 20.0},
			"output_tokens_details": map[string]interface{}{"reasoning_tokens": 10.0},
		},
	})
	if summary == nil {
		t.Fatal("usage summary is nil")
	}
	if summary.PromptTokens != 100 || summary.CompletionTokens != 50 {
		t.Fatalf("tokens = %+v", summary)
	}
	if summary.CachedTokens != 20 || summary.ReasoningTokens != 10 {
		t.Fatalf("details = %+v", summary)
	}
}

func TestExtractUsageDistinguishesExplicitZeroFromMissing(t *testing.T) {
	summary := ExtractUsage(map[string]interface{}{
		"usage": map[string]interface{}{
			"cache_read_input_tokens":   0.0,
			"prompt_tokens_details":     map[string]interface{}{"cached_tokens": 9.0},
			"completion_tokens_details": map[string]interface{}{"reasoning_tokens": 0.0},
			"output_tokens_details":     map[string]interface{}{"reasoning_tokens": 7.0},
		},
	})
	if summary == nil {
		t.Fatal("usage summary is nil")
	}
	if summary.CachedTokens != 0 || !summary.CachedTokensKnown {
		t.Fatalf("explicit cached zero = %+v", summary)
	}
	if summary.ReasoningTokens != 0 || !summary.ReasoningTokensKnown {
		t.Fatalf("explicit reasoning zero = %+v", summary)
	}
	missing := ExtractUsage(map[string]interface{}{"usage": map[string]interface{}{}})
	if missing.CachedTokensKnown || missing.ReasoningTokensKnown {
		t.Fatalf("missing details marked known = %+v", missing)
	}
	topLevel := ExtractUsage(map[string]interface{}{"usage": map[string]interface{}{"cached_tokens": 4.0}})
	if topLevel.CachedTokens != 4 || !topLevel.CachedTokensKnown {
		t.Fatalf("top-level cached tokens = %+v", topLevel)
	}
}

func TestSseUsageParserHandlesSplitFramesAndDone(t *testing.T) {
	parser := NewSseUsageParser()
	parser.Push([]byte("data: {\"usage\":{\"input_tokens\":8,\"output_tokens\":4,\"input_tokens_details\":{"))
	parser.Push([]byte("\"cached_tokens\":2},\"output_tokens_details\":{\"reasoning_tokens\":1}}}\n\ndata: [DONE]\n\n"))
	summary := parser.Finish()
	if summary == nil {
		t.Fatal("split SSE usage was not parsed")
	}
	if summary.PromptTokens != 8 || summary.CompletionTokens != 4 || summary.CachedTokens != 2 || summary.ReasoningTokens != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestUsageKnownFlagsPreservePartialAndRejectInvalidCounts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value interface{}
		known bool
	}{
		{"missing", nil, false}, {"zero", 0.0, true}, {"negative", -1.0, false}, {"fraction", 0.5, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := ExtractUsage(map[string]interface{}{"usage": map[string]interface{}{"prompt_tokens": tc.value, "completion_tokens": 0.0, "cached_tokens": tc.value}})
			prompt, completion, cached, reasoning := s.NullableTokens()
			if s.PromptTokensKnown != tc.known || s.CachedTokensKnown != tc.known || completion == nil || *completion != 0 || reasoning != nil {
				t.Fatalf("knowledge changed: %+v", s)
			}
			if tc.known {
				if prompt == nil || cached == nil || *prompt != 0 || *cached != 0 {
					t.Fatal("explicit zero lost")
				}
			} else if prompt != nil || cached != nil {
				t.Fatal("invalid/unknown counts fabricated")
			}
		})
	}
	p := NewSseUsageParser()
	p.Push([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":100,\"output_tokens\":0}}}\n\n"))
	s := p.Finish()
	if s == nil || s.PromptTokensKnown || !s.CompletionTokensKnown || s.CachedTokensKnown || s.ReasoningTokensKnown {
		t.Fatalf("partial SSE usage=%+v", s)
	}
}
