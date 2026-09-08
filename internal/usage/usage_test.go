package usage

import "testing"

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
