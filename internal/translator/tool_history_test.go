package translator

import "testing"

func TestChatToResponsesPreservesToolDefinitionsAndHistory(t *testing.T) {
	out := ChatToResponses(map[string]interface{}{
		"model": "audit-model",
		"tools": []interface{}{map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "read_file", "description": "Read a file", "parameters": map[string]interface{}{"type": "object"}}}},
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "Read README"},
			map[string]interface{}{"role": "assistant", "content": nil, "tool_calls": []interface{}{map[string]interface{}{"id": "call_1", "type": "function", "function": map[string]interface{}{"name": "read_file", "arguments": "{}"}}}},
			map[string]interface{}{"role": "tool", "tool_call_id": "call_1", "content": "file contents"},
		},
	})
	tools, _ := out["tools"].([]interface{})
	tool, _ := tools[0].(map[string]interface{})
	if tool["name"] != "read_file" {
		t.Errorf("Responses tool requires top-level name; got=%v", tool)
	}
	input, _ := out["input"].([]interface{})
	callFound, outputFound := false, false
	for _, raw := range input {
		item, _ := raw.(map[string]interface{})
		if item["type"] == "function_call" && item["call_id"] == "call_1" {
			callFound = true
		}
		if item["type"] == "function_call_output" && item["call_id"] == "call_1" {
			outputFound = true
		}
	}
	if !callFound || !outputFound {
		t.Errorf("tool history missing Responses call/output items: input=%v", input)
	}
}

func TestChatAnthropicPreservesToolCalls(t *testing.T) {
	request := OpenAIToAnthropic(map[string]interface{}{
		"model":    "audit-model",
		"tools":    []interface{}{map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "read_file", "parameters": map[string]interface{}{"type": "object"}}}},
		"messages": []interface{}{map[string]interface{}{"role": "user", "content": "Read README"}},
	})
	if _, ok := request["tools"]; !ok {
		t.Errorf("Chat -> Anthropic dropped tools: request=%v", request)
	}
	response := AnthropicToOpenAIResponse(map[string]interface{}{
		"model": "audit-model", "stop_reason": "tool_use",
		"content": []interface{}{map[string]interface{}{"type": "tool_use", "id": "call_1", "name": "read_file", "input": map[string]interface{}{"path": "README.md"}}},
	})
	choices, _ := response["choices"].([]interface{})
	choice, _ := choices[0].(map[string]interface{})
	message, _ := choice["message"].(map[string]interface{})
	if _, ok := message["tool_calls"]; !ok {
		t.Errorf("Anthropic -> Chat dropped tool call: response=%v", response)
	}
	reverse := AnthropicToChat(map[string]interface{}{
		"model":    "audit-model",
		"tools":    []interface{}{map[string]interface{}{"name": "read_file", "input_schema": map[string]interface{}{"type": "object"}}},
		"messages": []interface{}{map[string]interface{}{"role": "user", "content": "Read README"}},
	})
	if _, ok := reverse["tools"]; !ok {
		t.Errorf("Anthropic -> Chat dropped tools: request=%v", reverse)
	}
}

func TestToolHistoryRoundTrip(t *testing.T) {
	body := map[string]interface{}{
		"model": "m",
		"messages": []interface{}{
			map[string]interface{}{"role": "user", "content": "look up"},
			map[string]interface{}{"role": "assistant", "content": nil, "tool_calls": []interface{}{map[string]interface{}{
				"id": "call-7", "type": "function", "function": map[string]interface{}{"name": "lookup", "arguments": `{"q":"pi"}`},
			}}},
			map[string]interface{}{"role": "tool", "tool_call_id": "call-7", "content": "found"},
		},
		"tools":               []interface{}{map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "lookup", "parameters": map[string]interface{}{"type": "object"}}}},
		"tool_choice":         map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "lookup"}},
		"parallel_tool_calls": false,
	}
	for _, tc := range []struct{ api, reverseProto string }{{"openai-responses", "responses"}, {"anthropic-messages", "messages"}} {
		t.Run(tc.api, func(t *testing.T) {
			forward, err := PlanRequest("chat", tc.api, "auto")
			if err != nil {
				t.Fatal(err)
			}
			out, err := forward.TransformRequest("m", body)
			if err != nil {
				t.Fatal(err)
			}
			if input, ok := out["input"].([]interface{}); ok {
				for _, raw := range input {
					item := raw.(map[string]interface{})
					if item["role"] == "assistant" && item["content"] == nil {
						t.Fatal("nil content placeholder sent to Responses")
					}
				}
			}
			reverse, err := PlanRequest(tc.reverseProto, "openai-completions", "auto")
			if err != nil {
				t.Fatal(err)
			}
			got, err := reverse.TransformRequest("m", out)
			if err != nil {
				t.Fatal(err)
			}
			messages := got["messages"].([]interface{})
			if len(messages) != 3 {
				t.Fatalf("history=%v", messages)
			}
			calls := messages[1].(map[string]interface{})["tool_calls"].([]interface{})
			call := calls[0].(map[string]interface{})
			fn := call["function"].(map[string]interface{})
			result := messages[2].(map[string]interface{})
			if call["id"] != "call-7" || fn["name"] != "lookup" || fn["arguments"] != `{"q":"pi"}` || result["tool_call_id"] != "call-7" || result["content"] != "found" {
				t.Fatalf("exchange changed: %v", messages)
			}
			choice := got["tool_choice"].(map[string]interface{})
			if choice["function"].(map[string]interface{})["name"] != "lookup" || got["parallel_tool_calls"] != false {
				t.Fatalf("tool policy=%v", got)
			}
		})
	}
}

func TestInvalidToolHistoryFailsConversion(t *testing.T) {
	for _, api := range []string{"openai-responses", "anthropic-messages"} {
		plan, err := PlanRequest("chat", api, "auto")
		if err != nil {
			t.Fatal(err)
		}
		for _, body := range []map[string]interface{}{
			{"messages": []interface{}{map[string]interface{}{"role": "tool", "content": "result"}}},
			{"messages": []interface{}{map[string]interface{}{"role": "assistant", "tool_calls": "invalid"}}},
			{"tools": []interface{}{map[string]interface{}{"type": "custom", "name": "x"}}},
			{"tools": "invalid"},
		} {
			if _, err := plan.TransformRequest("m", body); err == nil {
				t.Errorf("%s accepted invalid tool history: %v", api, body)
			}
		}
	}
}

func TestResponsesInvalidToolHistoryFailsConversion(t *testing.T) {
	plan, err := PlanRequest("responses", "openai-completions", "auto")
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []map[string]interface{}{
		{"tools": []interface{}{"invalid"}},
		{"tools": []interface{}{map[string]interface{}{"type": "function"}}},
		{"input": []interface{}{map[string]interface{}{"type": "function_call", "name": "lookup", "arguments": "{}"}}},
		{"input": []interface{}{map[string]interface{}{"type": "function_call", "call_id": "c", "arguments": "{}"}}},
		{"input": []interface{}{map[string]interface{}{"type": "function_call", "call_id": "c", "name": "lookup", "arguments": "not-json"}}},
		{"input": []interface{}{map[string]interface{}{"type": "function_call_output", "output": "done"}}},
	} {
		if _, err := plan.TransformRequest("m", body); err == nil {
			t.Errorf("accepted invalid Responses tool history: %v", body)
		}
	}
}

func TestResponsesPureToolTurnHasNoEmptyMessage(t *testing.T) {
	plan, err := PlanRequest("chat", "openai-responses", "auto")
	if err != nil {
		t.Fatal(err)
	}
	for _, content := range []interface{}{nil, "", []interface{}{}} {
		out, err := plan.TransformRequest("m", map[string]interface{}{"messages": []interface{}{map[string]interface{}{
			"role": "assistant", "content": content, "tool_calls": []interface{}{map[string]interface{}{
				"id": "c", "type": "function", "function": map[string]interface{}{"name": "f", "arguments": "{}"},
			}},
		}}})
		if err != nil {
			t.Fatal(err)
		}
		input := out["input"].([]interface{})
		if len(input) != 1 || input[0].(map[string]interface{})["type"] != "function_call" {
			t.Errorf("pure tool turn has extra message: %v", input)
		}
	}
}
