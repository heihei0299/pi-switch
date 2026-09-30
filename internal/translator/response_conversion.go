package translator

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/heihei0299/pi-switch/internal/usage"
)

// OpenAIToAnthropicResponse converts Chat completion output to Messages output.
func OpenAIToAnthropicResponse(chat map[string]interface{}) (map[string]interface{}, error) {
	choices, ok := chat["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil, &ResponsesConversionError{Kind: "invalid", Message: "chat response has no choices array"}
	}
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return nil, &ResponsesConversionError{Kind: "invalid", Message: "chat response choice is invalid"}
	}
	message, _ := choice["message"].(map[string]interface{})
	if message == nil {
		return nil, &ResponsesConversionError{Kind: "invalid", Message: "chat response has no message"}
	}
	var content []interface{}
	if text, ok := message["content"].(string); ok && text != "" {
		content = append(content, map[string]interface{}{"type": "text", "text": text})
	}
	if calls, ok := message["tool_calls"].([]interface{}); ok {
		for _, raw := range calls {
			call, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			fn, _ := call["function"].(map[string]interface{})
			if fn == nil {
				fn = map[string]interface{}{}
			}
			input, err := decodeJSONValue(fn["arguments"])
			if err != nil {
				return nil, &ResponsesConversionError{Kind: "invalid", Message: fmt.Sprintf("tool call %v arguments are not valid JSON: %v", call["id"], err)}
			}
			id, _ := call["id"].(string)
			name, _ := fn["name"].(string)
			if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" {
				return nil, &ResponsesConversionError{Kind: "invalid", Message: "tool call is missing id or function name"}
			}
			content = append(content, map[string]interface{}{
				"type":  "tool_use",
				"id":    id,
				"name":  name,
				"input": input,
			})
		}
	}
	if content == nil {
		content = []interface{}{}
	}
	stopReason := "end_turn"
	if reason, _ := choice["finish_reason"].(string); reason != "" {
		switch reason {
		case "length":
			stopReason = "max_tokens"
		case "stop":
			stopReason = "end_turn"
		case "tool_calls", "function_call":
			stopReason = "tool_use"
		default:
			stopReason = reason
		}
	}
	model, _ := chat["model"].(string)
	resp := map[string]interface{}{
		"id":            chat["id"],
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"content":       content,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
	}
	if resp["id"] == nil {
		resp["id"] = fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	if summary := usage.ExtractUsage(chat); summary != nil {
		prompt, completion, cached, _ := summary.NullableTokens()
		mapped := map[string]interface{}{
			"input_tokens":  nil,
			"output_tokens": nil,
		}
		if prompt != nil {
			mapped["input_tokens"] = *prompt
		}
		if completion != nil {
			mapped["output_tokens"] = *completion
		}
		if prompt != nil && cached != nil {
			if *cached > *prompt {
				return nil, &ResponsesConversionError{Kind: "invalid", Message: "cached tokens exceed total input"}
			}
			mapped["input_tokens"] = *prompt - *cached
		}
		if cached != nil {
			mapped["cache_read_input_tokens"] = *cached
			// Chat input has already assigned all non-read tokens to input_tokens.
			mapped["cache_creation_input_tokens"] = int64(0)
		}
		resp["usage"] = mapped
	}
	return resp, nil
}

func decodeJSONValue(value interface{}) (interface{}, error) {
	text, ok := value.(string)
	if !ok {
		if value == nil {
			return nil, fmt.Errorf("value is missing")
		}
		return value, nil
	}
	var decoded interface{}
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

// ResponsesToChatResponse converts a Responses API completion into the Chat
// Completions response shape. It is deliberately separate from ResponsesToChat:
// the latter converts request input and cannot be used on an upstream response.
func ResponsesToChatResponse(body map[string]interface{}, fallbackModel string) (map[string]interface{}, error) {
	items, ok := body["output"].([]interface{})
	if !ok {
		return nil, &ResponsesConversionError{Kind: "invalid", Message: "responses response has no output array"}
	}
	var textParts []string
	var toolCalls []interface{}
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		typ, _ := item["type"].(string)
		switch typ {
		case "message":
			content, _ := item["content"].([]interface{})
			for _, rawPart := range content {
				part, ok := rawPart.(map[string]interface{})
				if !ok {
					continue
				}
				partType, _ := part["type"].(string)
				if partType == "output_text" || partType == "text" {
					if text, ok := part["text"].(string); ok {
						textParts = append(textParts, text)
					}
				}
			}
		case "function_call":
			callID, _ := item["call_id"].(string)
			name, _ := item["name"].(string)
			if strings.TrimSpace(callID) == "" || strings.TrimSpace(name) == "" {
				return nil, &ResponsesConversionError{Kind: "invalid", Message: "responses function call is missing call_id or name"}
			}
			arguments := item["arguments"]
			if argumentText, ok := arguments.(string); ok {
				arguments = argumentText
			} else {
				encoded, err := json.Marshal(arguments)
				if err != nil {
					return nil, &ResponsesConversionError{Kind: "invalid", Message: fmt.Sprintf("responses function call %s has invalid arguments: %v", callID, err)}
				}
				arguments = string(encoded)
			}
			toolCalls = append(toolCalls, map[string]interface{}{
				"id": callID, "type": "function",
				"function": map[string]interface{}{"name": name, "arguments": arguments},
			})
		}
	}
	message := map[string]interface{}{"role": "assistant", "content": strings.Join(textParts, "")}
	if len(textParts) == 0 {
		message["content"] = nil
	}
	finishReason := "stop"
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
		finishReason = "tool_calls"
	}
	if status, _ := body["status"].(string); status == "incomplete" {
		finishReason = "length"
		if details, ok := body["incomplete_details"].(map[string]interface{}); ok && details["reason"] == "content_filter" {
			finishReason = "content_filter"
		}
	}
	model, _ := body["model"].(string)
	if model == "" {
		model = fallbackModel
	}
	created := float64(time.Now().Unix())
	if value, ok := body["created_at"].(float64); ok {
		created = value
	}
	response := map[string]interface{}{
		"id":      body["id"],
		"object":  "chat.completion",
		"created": created,
		"model":   model,
		"choices": []interface{}{map[string]interface{}{
			"index": 0, "message": message, "finish_reason": finishReason,
		}},
	}
	if response["id"] == nil {
		response["id"] = fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	}
	if usage, ok := body["usage"].(map[string]interface{}); ok {
		mapped := map[string]interface{}{
			"prompt_tokens":     usage["input_tokens"],
			"completion_tokens": usage["output_tokens"],
			"total_tokens":      usage["total_tokens"],
		}
		if details, ok := usage["input_tokens_details"].(map[string]interface{}); ok {
			mapped["prompt_tokens_details"] = details
		}
		if details, ok := usage["output_tokens_details"].(map[string]interface{}); ok {
			mapped["completion_tokens_details"] = details
		}
		if mapped["total_tokens"] == nil {
			mapped["total_tokens"] = sumNumeric(mapped["prompt_tokens"], mapped["completion_tokens"])
		}
		response["usage"] = mapped
	}
	return response, nil
}

func sumNumeric(values ...interface{}) float64 {
	var total float64
	for _, value := range values {
		switch n := value.(type) {
		case float64:
			total += n
		case int:
			total += float64(n)
		case int64:
			total += float64(n)
		}
	}
	return total
}
