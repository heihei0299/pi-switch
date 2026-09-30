package translator_test

import (
	"github.com/heihei0299/pi-switch/internal/translator"
	"testing"
)

func TestResponsesIncompleteReasonReachesChat(t *testing.T) {
	for _, tc := range []struct{ reason, want string }{
		{"max_output_tokens", "length"}, {"content_filter", "content_filter"}, {"", "length"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			plan, err := translator.PlanRequest("chat", "openai-responses", "auto")
			if err != nil {
				t.Fatal(err)
			}
			got, err := plan.TransformResponse(map[string]interface{}{
				"status": "incomplete", "incomplete_details": map[string]interface{}{"reason": tc.reason},
				"output": []interface{}{map[string]interface{}{"type": "message", "content": []interface{}{map[string]interface{}{"type": "output_text", "text": "partial"}}}},
			}, "m")
			if err != nil {
				t.Fatal(err)
			}
			choices := got["choices"].([]interface{})
			if reason := choices[0].(map[string]interface{})["finish_reason"]; reason != tc.want {
				t.Fatalf("finish_reason=%v, want %s", reason, tc.want)
			}
		})
	}
}
