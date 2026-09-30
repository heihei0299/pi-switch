package translator

import "testing"

func TestChatAnthropicPreservesToolExchange(t *testing.T) {
	plan, err := PlanRequest("chat", "anthropic-messages", "auto")
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]interface{}{
		"model": "claude-test",
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "call the tool"},
			map[string]interface{}{
				"role": "assistant",
				"tool_calls": []interface{}{map[string]interface{}{
					"id": "call-1", "type": "function",
					"function": map[string]interface{}{"name": "lookup", "arguments": `{"q":"pi"}`},
				}},
			},
			map[string]interface{}{"role": "tool", "tool_call_id": "call-1", "content": `{"ok":true}`},
		},
		"tools": []interface{}{map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name": "lookup", "description": "look up data",
				"parameters": map[string]interface{}{"type": "object"},
			},
		}},
		"tool_choice": map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "lookup"}},
	}
	out, err := plan.TransformRequest("claude-test", body)
	if err != nil {
		t.Fatal(err)
	}
	msgs, _ := out["messages"].([]interface{})
	if len(msgs) != 3 {
		t.Fatalf("messages = %v", out["messages"])
	}
	assistant, _ := msgs[1].(map[string]interface{})
	assistantParts, _ := assistant["content"].([]interface{})
	if len(assistantParts) != 1 || assistantParts[0].(map[string]interface{})["type"] != "tool_use" {
		t.Fatalf("assistant tool_use = %v", assistant["content"])
	}
	toolResult, _ := msgs[2].(map[string]interface{})
	if toolResult["role"] != "user" {
		t.Fatalf("tool result role = %v", toolResult["role"])
	}
	tools, _ := out["tools"].([]interface{})
	tool, _ := tools[0].(map[string]interface{})
	if tool["name"] != "lookup" || tool["input_schema"] == nil {
		t.Fatalf("Anthropic tools = %v", out["tools"])
	}
	choice, _ := out["tool_choice"].(map[string]interface{})
	if choice["type"] != "tool" || choice["name"] != "lookup" {
		t.Fatalf("Anthropic tool_choice = %v", out["tool_choice"])
	}

	chatResponse, err := plan.TransformResponse(map[string]interface{}{
		"id": "msg-1", "model": "claude-test", "stop_reason": "tool_use",
		"content": []interface{}{map[string]interface{}{
			"type": "tool_use", "id": "call-2", "name": "lookup",
			"input": map[string]interface{}{"q": "pi"},
		}},
	}, "claude-test")
	if err != nil {
		t.Fatal(err)
	}
	choices, _ := chatResponse["choices"].([]interface{})
	message := choices[0].(map[string]interface{})["message"].(map[string]interface{})
	calls, _ := message["tool_calls"].([]interface{})
	if len(calls) != 1 || calls[0].(map[string]interface{})["id"] != "call-2" {
		t.Fatalf("Chat tool_calls = %v", message["tool_calls"])
	}
}

func TestAnthropicChatPreservesToolExchange(t *testing.T) {
	plan, err := PlanRequest("messages", "openai-completions", "auto")
	if err != nil {
		t.Fatal(err)
	}
	out, err := plan.TransformRequest("gpt-test", map[string]interface{}{
		"model": "gpt-test",
		"messages": []interface{}{
			map[string]interface{}{"role": "assistant", "content": []interface{}{map[string]interface{}{
				"type": "tool_use", "id": "call-1", "name": "lookup", "input": map[string]interface{}{"q": "pi"},
			}}},
			map[string]interface{}{"role": "user", "content": []interface{}{map[string]interface{}{
				"type": "tool_result", "tool_use_id": "call-1", "content": "done",
			}}},
		},
		"tools": []interface{}{map[string]interface{}{"name": "lookup", "input_schema": map[string]interface{}{"type": "object"}}},
		"tool_choice": map[string]interface{}{"type": "tool", "name": "lookup"},
	})
	if err != nil {
		t.Fatal(err)
	}
	msgs, _ := out["messages"].([]interface{})
	if len(msgs) != 2 {
		t.Fatalf("Chat messages = %v", out["messages"])
	}
	assistant := msgs[0].(map[string]interface{})
	calls, _ := assistant["tool_calls"].([]interface{})
	if len(calls) != 1 || calls[0].(map[string]interface{})["id"] != "call-1" {
		t.Fatalf("Chat tool_calls = %v", assistant["tool_calls"])
	}
	tool := msgs[1].(map[string]interface{})
	if tool["role"] != "tool" || tool["tool_call_id"] != "call-1" {
		t.Fatalf("Chat tool result = %v", tool)
	}

	response, err := plan.TransformResponse(map[string]interface{}{
		"id": "chat-1", "model": "gpt-test", "choices": []interface{}{map[string]interface{}{
			"message": map[string]interface{}{
				"role": "assistant", "tool_calls": []interface{}{map[string]interface{}{
					"id": "call-2", "function": map[string]interface{}{"name": "lookup", "arguments": `{"q":"pi"}`},
				}},
			},
			"finish_reason": "tool_calls",
		}},
	}, "gpt-test")
	if err != nil {
		t.Fatal(err)
	}
	content, _ := response["content"].([]interface{})
	if len(content) != 1 || content[0].(map[string]interface{})["type"] != "tool_use" {
		t.Fatalf("Anthropic response content = %v", response["content"])
	}
}

func TestOpenAIToAnthropicResponseRejectsInvalidToolArguments(t *testing.T) {
	_, err := OpenAIToAnthropicResponse(map[string]interface{}{
		"choices": []interface{}{map[string]interface{}{
			"message": map[string]interface{}{"tool_calls": []interface{}{map[string]interface{}{
				"id": "call-1", "function": map[string]interface{}{"name": "lookup", "arguments": "not-json"},
			}}},
		}},
	})
	if err == nil {
		t.Fatal("invalid tool arguments must fail conversion")
	}
}
