package translator

import "testing"

// RED: Responses-SSE-to-Chat-SSE converter (mirror of ChatSseToResponses).
func TestResponsesSseToChat_Stream(t *testing.T) {
	c := NewResponsesSseToChat("gpt-4o-mini")
	feed := []map[string]interface{}{
		{"type": "response.created", "response": map[string]interface{}{"id": "resp_1"}},
		{"type": "response.output_item.added", "output_index": float64(0),
			"item": map[string]interface{}{"id": "msg_1", "type": "message"}},
		{"type": "response.output_text.delta", "item_id": "msg_1",
			"output_index": float64(0), "content_index": float64(0), "delta": "hello"},
		{"type": "response.output_item.added", "output_index": float64(1),
			"item": map[string]interface{}{"id": "fc_1", "type": "function_call",
				"call_id": "call_1", "name": "get_time", "arguments": ""}},
		{"type": "response.function_call_arguments.delta", "item_id": "fc_1",
			"output_index": float64(1), "delta": "{}"},
		{"type": "response.completed", "response": map[string]interface{}{"id": "resp_1",
			"usage": map[string]interface{}{"input_tokens": float64(10),
				"output_tokens": float64(5), "total_tokens": float64(15)}}},
	}
	var chunks []map[string]interface{}
	for _, ev := range feed {
		out, err := c.PushEvent(ev)
		if err != nil {
			t.Fatalf("PushEvent: %v", err)
		}
		chunks = append(chunks, out...)
	}
	chunks = append(chunks, c.Finish()...)

	var sawText, sawTool bool
	var final map[string]interface{}
	for _, ch := range chunks {
		if ch["object"] != "chat.completion.chunk" {
			t.Fatalf("chunk object = %v, want chat.completion.chunk", ch["object"])
		}
		choices, _ := ch["choices"].([]interface{})
		if len(choices) == 0 {
			t.Fatalf("chunk has no choices: %v", ch)
		}
		first, _ := choices[0].(map[string]interface{})
		if fr, ok := first["finish_reason"]; ok && fr != nil {
			final = ch
			continue
		}
		delta, _ := first["delta"].(map[string]interface{})
		if delta == nil {
			continue
		}
		if delta["content"] == "hello" {
			sawText = true
		}
		if tcs, ok := delta["tool_calls"].([]interface{}); ok {
			for _, raw := range tcs {
				m, _ := raw.(map[string]interface{})
				fn, _ := m["function"].(map[string]interface{})
				if fn["name"] == "get_time" {
					sawTool = true
				}
				if args, _ := fn["arguments"].(string); args != "" && args != "{}" {
					t.Fatalf("unexpected arguments delta %q", args)
				}
			}
		}
	}
	if !sawText {
		t.Fatal("no text delta chunk with content hello")
	}
	if !sawTool {
		t.Fatal("no tool_calls chunk for get_time")
	}
	if final == nil {
		t.Fatal("no final chunk with finish_reason")
	}
	choices, _ := final["choices"].([]interface{})
	first, _ := choices[0].(map[string]interface{})
	if first["finish_reason"] != "tool_calls" {
		t.Fatalf("finish_reason = %v, want tool_calls", first["finish_reason"])
	}
	usage, _ := final["usage"].(map[string]interface{})
	if usage["prompt_tokens"] != float64(10) || usage["completion_tokens"] != float64(5) {
		t.Fatalf("final usage = %v, want prompt 10 / completion 5", usage)
	}
}

func TestResponsesSseToChat_FailedEndsStop(t *testing.T) {
	c := NewResponsesSseToChat("gpt-4o-mini")
	out, err := c.PushEvent(map[string]interface{}{
		"type":     "response.failed",
		"response": map[string]interface{}{"error": map[string]interface{}{"message": "boom"}},
	})
	if err != nil {
		t.Fatalf("PushEvent: %v", err)
	}
	out = append(out, c.Finish()...)
	found := false
	for _, ch := range out {
		choices, _ := ch["choices"].([]interface{})
		if len(choices) == 0 {
			continue
		}
		if first, _ := choices[0].(map[string]interface{}); first["finish_reason"] == "stop" {
			found = true
		}
	}
	if !found {
		t.Fatal("failed stream should finish with finish_reason stop")
	}
}

// RED c1: a failure AFTER tool calls must still end with stop, not tool_calls.
func TestResponsesSseToChat_FailedAfterToolsEndsStop(t *testing.T) {
	c := NewResponsesSseToChat("gpt-4o-mini")
	feed := []map[string]interface{}{
		{"type": "response.output_item.added", "output_index": float64(0),
			"item": map[string]interface{}{"id": "fc_1", "type": "function_call",
				"call_id": "call_1", "name": "get_time", "arguments": ""}},
		{"type": "response.failed",
			"response": map[string]interface{}{"error": map[string]interface{}{"message": "boom"}}},
	}
	var chunks []map[string]interface{}
	for _, ev := range feed {
		out, err := c.PushEvent(ev)
		if err != nil {
			t.Fatalf("PushEvent: %v", err)
		}
		chunks = append(chunks, out...)
	}
	chunks = append(chunks, c.Finish()...)
	var last map[string]interface{}
	for _, ch := range chunks {
		choices, _ := ch["choices"].([]interface{})
		if len(choices) == 0 {
			continue
		}
		if first, _ := choices[0].(map[string]interface{}); first["finish_reason"] != nil {
			last = ch
		}
	}
	if last == nil {
		t.Fatal("no terminal chunk")
	}
	choices, _ := last["choices"].([]interface{})
	first, _ := choices[0].(map[string]interface{})
	if first["finish_reason"] != "stop" {
		t.Fatalf("failed-after-tools finish_reason = %v, want stop", first["finish_reason"])
	}
}

// RED c2: arguments delta without a preceding added must keep its payload.
func TestResponsesSseToChat_OrphanArgumentsDeltaKept(t *testing.T) {
	c := NewResponsesSseToChat("gpt-4o-mini")
	out, err := c.PushEvent(map[string]interface{}{
		"type": "response.function_call_arguments.delta", "item_id": "fc_9",
		"output_index": float64(0), "delta": "{\"x\":1}",
	})
	if err != nil {
		t.Fatalf("PushEvent: %v", err)
	}
	joined := ""
	for _, ch := range out {
		choices, _ := ch["choices"].([]interface{})
		if len(choices) == 0 {
			continue
		}
		delta, _ := choices[0].(map[string]interface{})["delta"].(map[string]interface{})
		if delta == nil {
			continue
		}
		if tcs, ok := delta["tool_calls"].([]interface{}); ok {
			for _, raw := range tcs {
				m, _ := raw.(map[string]interface{})
				fn, _ := m["function"].(map[string]interface{})
				if args, _ := fn["arguments"].(string); args != "" {
					joined += args
				}
			}
		}
	}
	if joined != "{\"x\":1}" {
		t.Fatalf("orphan arguments delta lost, got %q", joined)
	}
}
