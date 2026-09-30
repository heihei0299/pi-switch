package translator

import (
	"testing"

	"github.com/heihei0299/pi-switch/internal/usage"
)

func TestAnthropicCacheTotalsSurviveResponseConversion(t *testing.T) {
	anthro := map[string]interface{}{
		"content": []interface{}{map[string]interface{}{"type": "text", "text": "hello"}},
		"usage":   map[string]interface{}{"input_tokens": 100, "cache_creation_input_tokens": 200, "cache_read_input_tokens": 700, "output_tokens": 50},
	}
	chat, err := AnthropicToOpenAIResponseWithError(anthro)
	if err != nil {
		t.Fatal(err)
	}
	s := usage.ExtractUsage(chat)
	if s == nil || s.PromptTokens != 1000 || s.CompletionTokens != 50 || s.CachedTokens != 700 {
		t.Fatalf("converted Chat usage=%+v", s)
	}
	mapped := chat["usage"].(map[string]interface{})
	if mapped["total_tokens"] != float64(1050) {
		t.Fatalf("total=%v", mapped["total_tokens"])
	}
	roundTrip, err := OpenAIToAnthropicResponse(chat)
	if err != nil {
		t.Fatal(err)
	}
	s = usage.ExtractUsage(roundTrip)
	if s == nil || s.PromptTokens != 1000 || s.CachedTokens != 700 {
		t.Fatalf("round-trip Anthropic usage=%+v", s)
	}
	raw := roundTrip["usage"].(map[string]interface{})
	if raw["input_tokens"] != int64(300) || raw["cache_read_input_tokens"] != int64(700) {
		t.Fatalf("Anthropic usage should exclude cache read from plain input: %v", raw)
	}
}
