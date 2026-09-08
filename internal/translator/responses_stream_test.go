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

func TestResponsesSseToChat_FailedEmitsErrorWithoutFinish(t *testing.T) {
	c := NewResponsesSseToChat("gpt-4o-mini")
	out, err := c.PushEvent(map[string]interface{}{
		"type":     "response.failed",
		"response": map[string]interface{}{"error": map[string]interface{}{"message": "boom"}},
	})
	if err != nil {
		t.Fatalf("PushEvent: %v", err)
	}
	out = append(out, c.Finish()...)
	var failure map[string]interface{}
	for _, event := range out {
		if raw, ok := event["error"].(map[string]interface{}); ok {
			failure = raw
		}
		if choices, ok := event["choices"].([]interface{}); ok && len(choices) > 0 {
			first, _ := choices[0].(map[string]interface{})
			if first["finish_reason"] != nil {
				t.Fatalf("failed stream emitted finish_reason %v", first["finish_reason"])
			}
		}
	}
	if failure == nil || failure["message"] != "boom" {
		t.Fatalf("failure event = %v, want upstream error", failure)
	}
}

// Contract: a failure after tool output must remain an error, not a successful tool_calls finish.
func TestResponsesSseToChat_FailedAfterToolsEmitsError(t *testing.T) {
	c := NewResponsesSseToChat("gpt-4o-mini")
	sawFailure := false
	for _, event := range []map[string]interface{}{
		{"type": "response.output_item.added", "item": map[string]interface{}{"id": "fc_1", "type": "function_call", "call_id": "call_1", "name": "get_time", "arguments": ""}},
		{"type": "response.failed", "response": map[string]interface{}{"error": map[string]interface{}{"message": "boom"}}},
	} {
		out, err := c.PushEvent(event)
		if err != nil {
			t.Fatalf("PushEvent: %v", err)
		}
		for _, chunk := range out {
			if raw, ok := chunk["error"].(map[string]interface{}); ok && raw["message"] == "boom" {
				sawFailure = true
			}
			if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
				first, _ := choices[0].(map[string]interface{})
				if first["finish_reason"] != nil {
					t.Fatalf("failed-after-tools emitted finish_reason %v", first["finish_reason"])
				}
			}
		}
	}
	if !sawFailure {
		t.Fatal("failed-after-tools did not emit upstream error")
	}
	out, err := c.PushEvent(map[string]interface{}{"type": "response.completed"})
	if err != nil {
		t.Fatalf("late completion: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("events after failed stream = %v", out)
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

// Contract: failed/incomplete/truncated Responses streams must not become a successful tool_calls finish.
func TestResponsesSseToChat_IncompleteUsesLengthFinish(t *testing.T) {
	c := NewResponsesSseToChat("gpt-4o-mini")
	out, err := c.PushEvent(map[string]interface{}{
		"type": "response.incomplete",
		"response": map[string]interface{}{
			"status":             "incomplete",
			"incomplete_details": map[string]interface{}{"reason": "max_output_tokens"},
		},
	})
	if err != nil {
		t.Fatalf("PushEvent: %v", err)
	}
	out = append(out, c.Finish()...)
	for _, chunk := range out {
		choices, _ := chunk["choices"].([]interface{})
		if len(choices) == 0 {
			continue
		}
		first, _ := choices[0].(map[string]interface{})
		if first["finish_reason"] != nil && first["finish_reason"] != "length" {
			t.Fatalf("finish_reason = %v, want length", first["finish_reason"])
		}
		if first["finish_reason"] == "length" {
			return
		}
	}
	t.Fatal("incomplete stream did not emit length finish_reason")
}

func TestResponsesSseToChat_ContentFilterFinish(t *testing.T) {
	c := NewResponsesSseToChat("gpt-4o-mini")
	out, err := c.PushEvent(map[string]interface{}{
		"type": "response.incomplete",
		"response": map[string]interface{}{
			"status":             "incomplete",
			"incomplete_details": map[string]interface{}{"reason": "content_filter"},
		},
	})
	if err != nil {
		t.Fatalf("PushEvent: %v", err)
	}
	for _, chunk := range out {
		choices, _ := chunk["choices"].([]interface{})
		if len(choices) == 0 {
			continue
		}
		first, _ := choices[0].(map[string]interface{})
		if first["finish_reason"] == "content_filter" {
			return
		}
	}
	t.Fatal("content_filter stream did not preserve finish_reason")
}

func TestResponsesSseToChat_MalformedIncompleteEmitsError(t *testing.T) {
	c := NewResponsesSseToChat("gpt-4o-mini")
	out, err := c.PushEvent(map[string]interface{}{"type": "response.incomplete"})
	if err != nil {
		t.Fatalf("PushEvent: %v", err)
	}
	if len(out) != 1 || out[0]["error"] == nil {
		t.Fatalf("malformed incomplete event = %v, want explicit error", out)
	}
}
