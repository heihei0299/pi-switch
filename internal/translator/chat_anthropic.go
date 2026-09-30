package translator

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ChatToResponses converts an OpenAI Chat Completions body into an OpenAI
// Responses body. Moved from internal/server (chatToResponses) so every
// conversion lives behind the translator registry.
func ChatToResponses(body map[string]interface{}) map[string]interface{} {
	out, _ := ChatToResponsesWithError(body)
	return out
}

// ChatToResponsesWithError converts Chat history including the function-call
// turns that are represented as separate items by the Responses API. The
// compatibility wrapper above is retained for callers that only need the
// historical best-effort conversion; request routing uses this error-returning
// form so malformed tool state is never silently discarded.
func ChatToResponsesWithError(body map[string]interface{}) (map[string]interface{}, error) {
	if err := validateToolDefinitions(body); err != nil {
		return nil, err
	}
	var input []interface{}
	var instructions []string
	if msgs, ok := body["messages"].([]interface{}); ok {
		for _, m := range msgs {
			if pm, ok := m.(map[string]interface{}); ok {
				role, _ := pm["role"].(string)
				if role == "system" || role == "developer" {
					if text := chatInstructionText(pm["content"]); text != "" {
						instructions = append(instructions, text)
					}
					continue
				}
				if role == "tool" {
					callID, _ := pm["tool_call_id"].(string)
					if strings.TrimSpace(callID) == "" {
						return nil, fmt.Errorf("tool message is missing tool_call_id")
					}
					output := pm["content"]
					if output == nil {
						output = ""
					}
					if _, ok := output.([]interface{}); ok {
						output = normalizeChatContent(output)
					}
					input = append(input, map[string]interface{}{
						"type": "function_call_output", "call_id": callID, "output": output,
					})
					continue
				}
				hasContent := pm["content"] != nil
				if role == "assistant" && pm["tool_calls"] != nil {
					switch content := pm["content"].(type) {
					case string:
						hasContent = content != ""
					case []interface{}:
						hasContent = len(content) > 0
					}
				}
				if hasContent {
					content := normalizeChatContent(pm["content"])
					input = append(input, map[string]interface{}{"role": role, "content": content})
				}
				if role == "assistant" {
					calls, ok := pm["tool_calls"].([]interface{})
					if !ok {
						if pm["tool_calls"] != nil {
							return nil, fmt.Errorf("assistant tool_calls is invalid")
						}
						continue
					}
					for _, rawCall := range calls {
						call, ok := rawCall.(map[string]interface{})
						if !ok {
							return nil, fmt.Errorf("assistant tool call is invalid")
						}
						callID, _ := call["id"].(string)
						fn, _ := call["function"].(map[string]interface{})
						if strings.TrimSpace(callID) == "" || fn == nil {
							return nil, fmt.Errorf("assistant tool call is missing id or function")
						}
						name, _ := fn["name"].(string)
						if strings.TrimSpace(name) == "" {
							return nil, fmt.Errorf("assistant tool call is missing function name")
						}
						arguments, err := encodeToolArguments(fn["arguments"])
						if err != nil {
							return nil, fmt.Errorf("tool call %s arguments: %w", callID, err)
						}
						input = append(input, map[string]interface{}{
							"type": "function_call", "call_id": callID, "name": name, "arguments": arguments,
						})
					}
				}
			}
		}
	}
	out := map[string]interface{}{
		"model": body["model"],
		"input": input,
	}
	if v, ok := body["max_tokens"]; ok {
		out["max_output_tokens"] = v
	}
	for _, k := range []string{"temperature", "top_p", "stream", "stop", "parallel_tool_calls"} {
		if v, ok := body[k]; ok {
			out[k] = v
		}
	}
	if len(instructions) > 0 {
		out["instructions"] = strings.Join(instructions, "\n")
	}
	if tools, ok := body["tools"].([]interface{}); ok {
		converted, err := chatToolsToResponses(tools)
		if err != nil {
			return nil, err
		}
		out["tools"] = converted
	}
	if choice, ok := body["tool_choice"]; ok {
		converted, err := chatToolChoiceToResponses(choice)
		if err != nil {
			return nil, err
		}
		out["tool_choice"] = converted
	}
	return out, nil
}

func chatToolsToResponses(tools []interface{}) ([]interface{}, error) {
	converted := make([]interface{}, 0, len(tools))
	for _, raw := range tools {
		tool, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("tool definition is invalid")
		}
		typ, _ := tool["type"].(string)
		if typ == "" {
			typ = "function"
		}
		if typ != "function" {
			return nil, fmt.Errorf("tool type %q is not supported by Responses", typ)
		}
		flat := map[string]interface{}{"type": "function"}
		if fn, ok := tool["function"].(map[string]interface{}); ok {
			for _, key := range []string{"name", "description", "parameters", "strict"} {
				if value, exists := fn[key]; exists {
					flat[key] = value
				}
			}
		} else {
			for _, key := range []string{"name", "description", "parameters", "strict"} {
				if value, exists := tool[key]; exists {
					flat[key] = value
				}
			}
		}
		if name, _ := flat["name"].(string); strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("function tool is missing name")
		}
		converted = append(converted, flat)
	}
	return converted, nil
}

func chatToolChoiceToResponses(choice interface{}) (interface{}, error) {
	if value, ok := choice.(string); ok {
		return value, nil
	}
	tool, ok := choice.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("tool_choice is invalid")
	}
	if typ, _ := tool["type"].(string); typ != "function" {
		return nil, fmt.Errorf("tool_choice type %q is not supported by Responses", typ)
	}
	name := ""
	if fn, ok := tool["function"].(map[string]interface{}); ok {
		name, _ = fn["name"].(string)
	} else {
		name, _ = tool["name"].(string)
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("tool_choice function is missing name")
	}
	return map[string]interface{}{"type": "function", "name": name}, nil
}

func chatInstructionText(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		texts := make([]string, 0, len(v))
		for _, raw := range v {
			part, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if text, ok := part["text"].(string); ok && text != "" {
				texts = append(texts, text)
			}
		}
		return strings.Join(texts, "\n")
	default:
		return ""
	}
}

func normalizeChatContent(content interface{}) interface{} {
	if s, ok := content.(string); ok {
		return []interface{}{map[string]interface{}{"type": "input_text", "text": s}}
	}
	arr, ok := content.([]interface{})
	if !ok {
		return content
	}
	out := make([]interface{}, 0, len(arr))
	for _, part := range arr {
		pm, ok := part.(map[string]interface{})
		if !ok {
			out = append(out, part)
			continue
		}
		typ, _ := pm["type"].(string)
		switch typ {
		case "text":
			t, _ := pm["text"].(string)
			out = append(out, map[string]interface{}{"type": "input_text", "text": t})
		case "image_url":
			var url string
			switch v := pm["image_url"].(type) {
			case string:
				url = v
			case map[string]interface{}:
				if u, ok := v["url"].(string); ok {
					url = u
				}
			}
			if url == "" {
				if u, ok := pm["url"].(string); ok {
					url = u
				}
			}
			out = append(out, map[string]interface{}{"type": "input_image", "image_url": url})
		case "input_text", "input_image", "output_text":
			// already Responses shaped or legacy, normalize output_text as well
			if typ == "output_text" {
				t, _ := pm["text"].(string)
				out = append(out, map[string]interface{}{"type": "input_text", "text": t})
			} else {
				out = append(out, pm)
			}
		default:
			out = append(out, pm)
		}
	}
	return out
}

// AnthropicToChat converts an Anthropic Messages body into an OpenAI Chat
// Completions body. Moved from internal/server (anthropicToChat) so every
// conversion lives behind the translator registry.
func AnthropicToChat(body map[string]interface{}) map[string]interface{} {
	out, _ := AnthropicToChatWithError(body)
	return out
}

func AnthropicToChatWithError(body map[string]interface{}) (map[string]interface{}, error) {
	if err := validateToolDefinitions(body); err != nil {
		return nil, err
	}
	model, _ := body["model"].(string)
	anthMsgs, _ := body["messages"].([]interface{})
	var chatMsgs []interface{}
	if system, ok := body["system"]; ok {
		text := chatInstructionText(system)
		if text != "" {
			chatMsgs = append(chatMsgs, map[string]interface{}{"role": "system", "content": text})
		}
	}
	for i, raw := range anthMsgs {
		pm, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("message %d is invalid", i)
		}
		role, _ := pm["role"].(string)
		if role != "user" && role != "assistant" {
			return nil, fmt.Errorf("message %d: unsupported Anthropic role %q", i, role)
		}
		if err := appendAnthropicContent(&chatMsgs, role, pm["content"], i); err != nil {
			return nil, err
		}
	}
	out := map[string]interface{}{"model": model, "messages": chatMsgs}
	for _, key := range []string{"max_tokens", "temperature", "stream"} {
		if value, ok := body[key]; ok {
			out[key] = value
		}
	}
	if value, ok := body["stop_sequences"]; ok {
		out["stop"] = value
	}
	if tools, ok := body["tools"].([]interface{}); ok {
		converted, err := anthropicToolsToChat(tools)
		if err != nil {
			return nil, err
		}
		out["tools"] = converted
	}
	if choice, ok := body["tool_choice"]; ok {
		converted, err := anthropicToolChoiceToChat(choice)
		if err != nil {
			return nil, err
		}
		out["tool_choice"] = converted
	}
	if choice, ok := body["tool_choice"].(map[string]interface{}); ok {
		if disable, ok := choice["disable_parallel_tool_use"].(bool); ok {
			out["parallel_tool_calls"] = !disable
		}
	}
	if chatMsgs == nil {
		out["messages"] = []interface{}{}
	}
	return out, nil
}

func chatContentToAnthropic(content interface{}, assistant bool) ([]interface{}, error) {
	var parts []interface{}
	appendText := func(text string) { parts = append(parts, map[string]interface{}{"type": "text", "text": text}) }
	switch value := content.(type) {
	case nil:
	case string:
		appendText(value)
	case []interface{}:
		for _, raw := range value {
			part, ok := raw.(map[string]interface{})
			if !ok {
				appendText(fmt.Sprintf("%v", raw))
				continue
			}
			typ, _ := part["type"].(string)
			switch typ {
			case "text", "input_text":
				if text, ok := part["text"].(string); ok {
					appendText(text)
				}
			default:
				if assistant && (typ == "tool_use" || typ == "tool_result") {
					return nil, fmt.Errorf("Chat assistant content contains unsupported %q block", typ)
				}
				appendText(fmt.Sprintf("%v", raw))
			}
		}
	default:
		appendText(fmt.Sprintf("%v", value))
	}
	return parts, nil
}

func chatToolCallsToAnthropic(raw interface{}) ([]interface{}, error) {
	if raw == nil {
		return nil, nil
	}
	calls, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("tool_calls is invalid")
	}
	converted := make([]interface{}, 0, len(calls))
	for i, item := range calls {
		call, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("tool call %d is invalid", i)
		}
		id, _ := call["id"].(string)
		fn, _ := call["function"].(map[string]interface{})
		if strings.TrimSpace(id) == "" || fn == nil {
			return nil, fmt.Errorf("tool call %d is missing id or function", i)
		}
		name, _ := fn["name"].(string)
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("tool call %d is missing function name", i)
		}
		input, err := decodeJSONValue(fn["arguments"])
		if err != nil {
			return nil, fmt.Errorf("tool call %s arguments are not valid JSON: %w", id, err)
		}
		converted = append(converted, map[string]interface{}{
			"type": "tool_use", "id": id, "name": name, "input": input,
		})
	}
	return converted, nil
}

func chatToolsToAnthropic(tools []interface{}) ([]interface{}, error) {
	converted := make([]interface{}, 0, len(tools))
	for i, raw := range tools {
		tool, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("tool %d is invalid", i)
		}
		typ, _ := tool["type"].(string)
		if typ == "" {
			typ = "function"
		}
		if typ != "function" {
			return nil, fmt.Errorf("tool %d type %q is not supported by Anthropic", i, typ)
		}
		fn, _ := tool["function"].(map[string]interface{})
		if fn == nil {
			fn = tool
		}
		name, _ := fn["name"].(string)
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("tool %d is missing function name", i)
		}
		schema := fn["parameters"]
		if schema == nil {
			schema = map[string]interface{}{"type": "object"}
		}
		entry := map[string]interface{}{"name": name, "input_schema": schema}
		if description, ok := fn["description"].(string); ok && description != "" {
			entry["description"] = description
		}
		converted = append(converted, entry)
	}
	return converted, nil
}

func chatToolChoiceToAnthropic(choice interface{}) (interface{}, error) {
	switch value := choice.(type) {
	case string:
		switch value {
		case "auto", "none":
			return map[string]interface{}{"type": value}, nil
		case "required":
			return map[string]interface{}{"type": "any"}, nil
		default:
			return nil, fmt.Errorf("tool_choice %q is not supported by Anthropic", value)
		}
	case map[string]interface{}:
		fn, _ := value["function"].(map[string]interface{})
		if fn == nil {
			fn = value
		}
		name, _ := fn["name"].(string)
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("tool_choice function is missing name")
		}
		return map[string]interface{}{"type": "tool", "name": name}, nil
	default:
		return nil, fmt.Errorf("tool_choice is invalid")
	}
}

func anthropicToolsToChat(tools []interface{}) ([]interface{}, error) {
	converted := make([]interface{}, 0, len(tools))
	for i, raw := range tools {
		tool, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("Anthropic tool %d is invalid", i)
		}
		name, _ := tool["name"].(string)
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("Anthropic tool %d is missing name", i)
		}
		parameters := tool["input_schema"]
		if parameters == nil {
			parameters = map[string]interface{}{"type": "object"}
		}
		fn := map[string]interface{}{"name": name, "parameters": parameters}
		if description, ok := tool["description"].(string); ok && description != "" {
			fn["description"] = description
		}
		converted = append(converted, map[string]interface{}{"type": "function", "function": fn})
	}
	return converted, nil
}

func anthropicToolChoiceToChat(choice interface{}) (interface{}, error) {
	value, ok := choice.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("Anthropic tool_choice is invalid")
	}
	typ, _ := value["type"].(string)
	switch typ {
	case "auto", "none":
		return typ, nil
	case "any":
		return "required", nil
	case "tool":
		name, _ := value["name"].(string)
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("Anthropic tool_choice tool is missing name")
		}
		return map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": name}}, nil
	default:
		return nil, fmt.Errorf("Anthropic tool_choice type %q is not supported by Chat", typ)
	}
}

func appendAnthropicContent(messages *[]interface{}, role string, content interface{}, index int) error {
	if text, ok := content.(string); ok {
		*messages = append(*messages, map[string]interface{}{"role": role, "content": text})
		return nil
	}
	parts, ok := content.([]interface{})
	if !ok {
		*messages = append(*messages, map[string]interface{}{"role": role, "content": fmt.Sprintf("%v", content)})
		return nil
	}
	var textParts []string
	var toolCalls []interface{}
	flushAssistant := func() {
		if role == "assistant" && (len(textParts) > 0 || len(toolCalls) > 0) {
			message := map[string]interface{}{"role": "assistant", "content": strings.Join(textParts, "")}
			if len(toolCalls) > 0 {
				message["tool_calls"] = toolCalls
			}
			*messages = append(*messages, message)
			textParts = nil
			toolCalls = nil
		}
	}
	for partIndex, raw := range parts {
		part, ok := raw.(map[string]interface{})
		if !ok {
			textParts = append(textParts, fmt.Sprintf("%v", raw))
			continue
		}
		typ, _ := part["type"].(string)
		switch typ {
		case "text":
			if text, ok := part["text"].(string); ok {
				textParts = append(textParts, text)
			}
		case "tool_use":
			if role != "assistant" {
				return fmt.Errorf("message %d block %d: tool_use must be assistant content", index, partIndex)
			}
			id, _ := part["id"].(string)
			name, _ := part["name"].(string)
			if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" {
				return fmt.Errorf("message %d block %d: tool_use is missing id or name", index, partIndex)
			}
			args, err := json.Marshal(part["input"])
			if err != nil {
				return fmt.Errorf("message %d block %d: tool_use input: %w", index, partIndex, err)
			}
			toolCalls = append(toolCalls, map[string]interface{}{
				"id": id, "type": "function",
				"function": map[string]interface{}{"name": name, "arguments": string(args)},
			})
		case "tool_result":
			if role != "user" {
				return fmt.Errorf("message %d block %d: tool_result must be user content", index, partIndex)
			}
			if len(textParts) > 0 {
				*messages = append(*messages, map[string]interface{}{"role": "user", "content": strings.Join(textParts, "")})
				textParts = nil
			}
			callID, _ := part["tool_use_id"].(string)
			if strings.TrimSpace(callID) == "" {
				return fmt.Errorf("message %d block %d: tool_result is missing tool_use_id", index, partIndex)
			}
			result := part["content"]
			if result == nil {
				result = ""
			} else if _, ok := result.(string); !ok {
				encoded, err := json.Marshal(result)
				if err != nil {
					return fmt.Errorf("message %d block %d: tool_result content: %w", index, partIndex, err)
				}
				result = string(encoded)
			}
			*messages = append(*messages, map[string]interface{}{"role": "tool", "tool_call_id": callID, "content": result})
		default:
			return fmt.Errorf("message %d block %d: unsupported Anthropic content type %q", index, partIndex, typ)
		}
	}
	if role == "assistant" {
		flushAssistant()
	} else if len(textParts) > 0 {
		*messages = append(*messages, map[string]interface{}{"role": role, "content": strings.Join(textParts, "")})
	}
	return nil
}

// All supported protocols define tools as an array; reject wrong shapes before
// conversion so a malformed declaration never turns into a request without tools.
func validateToolDefinitions(body map[string]interface{}) error {
	if raw, exists := body["tools"]; exists {
		if _, ok := raw.([]interface{}); !ok {
			return fmt.Errorf("tools must be an array")
		}
	}
	return nil
}

func encodeToolArguments(value interface{}) (string, error) {
	decoded, err := decodeJSONValue(value)
	if err != nil {
		return "", err
	}
	if text, ok := value.(string); ok {
		return text, nil
	}
	encoded, err := json.Marshal(decoded)
	return string(encoded), err
}
