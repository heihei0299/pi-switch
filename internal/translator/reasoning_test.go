package translator

import (
	"strings"
	"testing"
)

// RED: reasoning_content must survive Chat -> Responses conversion instead
// of being silently dropped (manual-test-basic on DeepSeek thinking output).
func TestChatResponseToResponses_ReasoningOnly(t *testing.T) {
	chat := map[string]interface{}{
		"id": "chatcmpl-1",
		"choices": []interface{}{map[string]interface{}{
			"message": map[string]interface{}{"role": "assistant", "content": "",
				"reasoning_content": "We need answer ping."},
		}},
	}
	resp, err := ChatResponseToResponses(chat, "deepseek-v4-flash", nil)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	out, _ := resp["output"].([]interface{})
	if len(out) != 1 {
		t.Fatalf("reasoning-only must yield 1 message item, got %v", out)
	}
	item, _ := out[0].(map[string]interface{})
	content, _ := item["content"].([]interface{})
	if len(content) == 0 {
		t.Fatalf("message item has no content parts: %v", item)
	}
	joined := ""
	for _, p := range content {
		if pm, ok := p.(map[string]interface{}); ok {
			if s, ok := pm["text"].(string); ok {
				joined += s
			}
		}
	}
	if !strings.Contains(joined, "We need answer ping.") {
		t.Fatalf("reasoning text lost, parts = %v", content)
	}
}

func TestChatResponseToResponses_ReasoningAndContent(t *testing.T) {
	chat := map[string]interface{}{
		"id": "chatcmpl-2",
		"choices": []interface{}{map[string]interface{}{
			"message": map[string]interface{}{"role": "assistant", "content": "pong",
				"reasoning_content": "thinking"},
		}},
	}
	resp, err := ChatResponseToResponses(chat, "m", nil)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	out, _ := resp["output"].([]interface{})
	if len(out) != 1 {
		t.Fatalf("want 1 message item, got %v", out)
	}
	item, _ := out[0].(map[string]interface{})
	parts, _ := item["content"].([]interface{})
	if len(parts) != 2 {
		t.Fatalf("want reasoning + content parts, got %v", parts)
	}
}

func TestChatSseToResponses_ReasoningDelta(t *testing.T) {
	c := NewChatSseToResponses("m")
	events, err := c.PushFrame(map[string]interface{}{
		"choices": []interface{}{map[string]interface{}{
			"delta": map[string]interface{}{"reasoning_content": "r1"},
		}},
	})
	if err != nil {
		t.Fatalf("PushFrame: %v", err)
	}
	saw := false
	for _, ev := range events {
		if ev["type"] == "response.output_text.delta" && ev["delta"] == "r1" {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("reasoning delta not emitted, events = %v", events)
	}
	if c.Text != "r1" {
		t.Fatalf("Text = %q, want r1", c.Text)
	}
}

func TestChatSseToResponses_ReasoningAndContentRemainOutputText(t *testing.T) {
	c := NewChatSseToResponses("m")
	for _, delta := range []map[string]interface{}{
		{"reasoning_content": "think"},
		{"content": "answer"},
	} {
		if _, err := c.PushFrame(map[string]interface{}{
			"choices": []interface{}{map[string]interface{}{"delta": delta}},
		}); err != nil {
			t.Fatalf("PushFrame: %v", err)
		}
	}
	joined := ""
	for _, event := range c.Finish() {
		if event["type"] != "response.completed" {
			continue
		}
		response, _ := event["response"].(map[string]interface{})
		output, _ := response["output"].([]interface{})
		if len(output) == 0 {
			t.Fatal("reasoning/content stream produced empty output")
		}
		item, _ := output[0].(map[string]interface{})
		parts, _ := item["content"].([]interface{})
		for _, raw := range parts {
			part, _ := raw.(map[string]interface{})
			if text, ok := part["text"].(string); ok {
				joined += text
			}
		}
	}
	if joined != "thinkanswer" {
		t.Fatalf("output text = %q, want thinkanswer", joined)
	}
}
