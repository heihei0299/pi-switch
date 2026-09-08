package translator

import (
	"encoding/json"
	"fmt"
	"time"
)

type ResponsesConversionError struct {
	Kind    string
	Message string
}

func (e *ResponsesConversionError) Error() string { return e.Message }

func ValidateResponsesMode(api, mode string) error {
	if mode == "" {
		mode = "auto"
	}
	if mode == "auto" {
		return nil
	}
	if mode == "passthrough" && api != "openai-responses" {
		return fmt.Errorf("responsesMode passthrough requires api openai-responses, got %s", api)
	}
	if mode == "convert" && api != "openai-completions" {
		return fmt.Errorf("responsesMode convert requires api openai-completions, got %s", api)
	}
	if mode != "passthrough" && mode != "convert" {
		return fmt.Errorf("invalid responsesMode %q", mode)
	}
	return nil
}

func IsNativeResponsesPassthrough(api, mode string) bool {
	if api != "openai-responses" {
		return false
	}
	return mode == "auto" || mode == "passthrough"
}

func IsChatConvert(api, mode string) bool {
	if api != "openai-completions" {
		return false
	}
	return mode == "auto" || mode == "convert"
}

func OpenAIToAnthropic(body map[string]interface{}) map[string]interface{} {
	model, _ := body["model"].(string)
	if model == "" {
		model = "claude-sonnet-4-5"
	}
	maxTokens := 16384
	if v, ok := body["max_tokens"].(float64); ok {
		maxTokens = int(v)
	}
	var messages []interface{}
	if m, ok := body["messages"].([]interface{}); ok {
		messages = m
	}
	var systemParts []interface{}
	var anthMsgs []interface{}
	for _, mi := range messages {
		msg, ok := mi.(map[string]interface{})
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if role == "system" {
			content := msg["content"]
			var text string
			switch c := content.(type) {
			case string:
				text = c
			case []interface{}:
				for _, part := range c {
					if pm, ok := part.(map[string]interface{}); ok {
						if t, ok := pm["text"].(string); ok {
							if text != "" {
								text += "\n"
							}
							text += t
						}
					}
				}
			default:
				if content != nil {
					b, _ := json.Marshal(content)
					text = string(b)
				}
			}
			if text != "" {
				systemParts = append(systemParts, map[string]interface{}{"type": "text", "text": text})
			}
		} else {
			newRole := "user"
			if role == "assistant" {
				newRole = "assistant"
			}
			content := msg["content"]
			if content == nil {
				content = ""
			}
			var parts []interface{}
			switch c := content.(type) {
			case string:
				parts = []interface{}{map[string]interface{}{"type": "text", "text": c}}
			case []interface{}:
				for _, cc := range c {
					if pm, ok := cc.(map[string]interface{}); ok {
						typ, _ := pm["type"].(string)
						if typ == "text" {
							t, _ := pm["text"].(string)
							parts = append(parts, map[string]interface{}{"type": "text", "text": t})
						} else {
							parts = append(parts, map[string]interface{}{"type": "text", "text": fmt.Sprintf("%v", cc)})
						}
					}
				}
			default:
				parts = []interface{}{map[string]interface{}{"type": "text", "text": fmt.Sprintf("%v", c)}}
			}
			anthMsgs = append(anthMsgs, map[string]interface{}{"role": newRole, "content": parts})
		}
	}
	if anthMsgs == nil {
		anthMsgs = []interface{}{}
	}
	out := map[string]interface{}{
		"model":      model,
		"max_tokens": maxTokens,
		"messages":   anthMsgs,
	}
	if len(systemParts) > 0 {
		out["system"] = systemParts
	}
	if v, ok := body["temperature"]; ok {
		out["temperature"] = v
	}
	if v, ok := body["stop"]; ok {
		switch s := v.(type) {
		case []interface{}:
			out["stop_sequences"] = s
		default:
			out["stop_sequences"] = []interface{}{s}
		}
	}
	if v, ok := body["stream"]; ok {
		out["stream"] = v
	}
	return out
}

func AnthropicToOpenAIResponse(anthro map[string]interface{}) map[string]interface{} {
	model, _ := anthro["model"].(string)
	if model == "" {
		model = "claude-sonnet-4-5"
	}
	contentBlocks, _ := anthro["content"].([]interface{})
	var choices []interface{}
	for i, block := range contentBlocks {
		text := ""
		if m, ok := block.(map[string]interface{}); ok {
			if t, ok := m["text"].(string); ok {
				text = t
			}
		}
		stopReason := "stop"
		if r, ok := anthro["stop_reason"].(string); ok {
			switch r {
			case "end_turn":
				stopReason = "stop"
			case "max_tokens":
				stopReason = "length"
			default:
				stopReason = r
			}
		}
		choices = append(choices, map[string]interface{}{
			"index": i,
			"message": map[string]interface{}{
				"role": "assistant", "content": text,
			},
			"finish_reason": stopReason,
		})
	}
	if choices == nil {
		choices = []interface{}{}
	}
	resp := map[string]interface{}{
		"id":      anthro["id"],
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": choices,
	}
	if resp["id"] == nil {
		resp["id"] = fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	}
	if usage, ok := anthro["usage"].(map[string]interface{}); ok {
		prompt := usage["input_tokens"]
		compl := usage["output_tokens"]
		var total float64
		if pf, ok := prompt.(float64); ok {
			total += pf
		}
		if cf, ok := compl.(float64); ok {
			total += cf
		}
		resp["usage"] = map[string]interface{}{
			"prompt_tokens":     prompt,
			"completion_tokens": compl,
			"total_tokens":      total,
		}
	}
	return resp
}

func ResponsesToChat(body map[string]interface{}) (map[string]interface{}, error) {
	var messages []interface{}
	if input, ok := body["input"]; ok {
		switch v := input.(type) {
		case []interface{}:
			conv, err := convertResponsesInput(v)
			if err != nil {
				return nil, err
			}
			messages = conv
		case string:
			messages = []interface{}{map[string]interface{}{"role": "user", "content": v}}
		default:
			messages = []interface{}{}
		}
	}
	chat := map[string]interface{}{
		"model":    body["model"],
		"messages": messages,
	}
	if chat["model"] == nil {
		chat["model"] = "gpt-4o-mini"
	}
	if v, ok := body["max_output_tokens"]; ok {
		chat["max_tokens"] = v
	} else if v, ok := body["max_tokens"]; ok {
		chat["max_tokens"] = v
	}
	for _, k := range []string{"temperature", "top_p", "stream", "stop"} {
		if v, ok := body[k]; ok {
			chat[k] = v
		}
	}
	if tools, ok := body["tools"].([]interface{}); ok {
		var chatTools []interface{}
		for _, t := range tools {
			m, ok := t.(map[string]interface{})
			if !ok {
				continue
			}
			typ, _ := m["type"].(string)
			if typ == "" {
				typ = "function"
			}
			if typ != "function" {
				return nil, &ResponsesConversionError{Kind: "not_supported", Message: fmt.Sprintf("tool type '%s' is not supported in conversion mode", typ)}
			}
			chatTools = append(chatTools, map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        m["name"],
					"description": m["description"],
					"parameters":  m["parameters"],
				},
			})
		}
		chat["tools"] = chatTools
		if v, ok := body["tool_choice"]; ok {
			chat["tool_choice"] = v
		}
	}
	if instr, ok := body["instructions"].(string); ok && instr != "" {
		msgs := chat["messages"].([]interface{})
		newMsgs := []interface{}{map[string]interface{}{"role": "system", "content": instr}}
		newMsgs = append(newMsgs, msgs...)
		chat["messages"] = newMsgs
	}
	if chat["messages"] == nil {
		chat["messages"] = []interface{}{}
	}
	return chat, nil
}

func convertResponsesInput(items []interface{}) ([]interface{}, error) {
	var messages []interface{}
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		switch typ {
		case "function_call_output":
			callID, _ := m["call_id"].(string)
			output := m["output"]
			messages = append(messages, map[string]interface{}{
				"role": "tool", "tool_call_id": callID, "content": output,
			})
		case "function_call":
			call := map[string]interface{}{
				"id":   m["call_id"],
				"type": "function",
				"function": map[string]interface{}{
					"name":      m["name"],
					"arguments": m["arguments"],
				},
			}
			if len(messages) > 0 {
				if last, ok := messages[len(messages)-1].(map[string]interface{}); ok {
					if last["role"] == "assistant" {
						if tc, ok := last["tool_calls"].([]interface{}); ok {
							last["tool_calls"] = append(tc, call)
							continue
						} else if last["tool_calls"] == nil {
							last["tool_calls"] = []interface{}{call}
							continue
						}
					}
				}
			}
			messages = append(messages, map[string]interface{}{
				"role": "assistant", "content": nil, "tool_calls": []interface{}{call},
			})
		default:
			role, _ := m["role"].(string)
			if role == "" {
				role = "user"
			}
			content := normalizeResponsesContent(m["content"])
			msg := map[string]interface{}{"role": role, "content": content}
			if role == "assistant" {
				if calls := extractEmbeddedFunctionCalls(&msg); calls != nil {
					msg["tool_calls"] = calls
				}
			}
			messages = append(messages, msg)
		}
	}
	return messages, nil
}
func normalizeResponsesContent(content interface{}) interface{} {
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
		case "input_text":
			t, _ := pm["text"].(string)
			out = append(out, map[string]interface{}{"type": "text", "text": t})
		case "input_image":
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
			out = append(out, map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": url}})
		case "output_text":
			t, _ := pm["text"].(string)
			out = append(out, map[string]interface{}{"type": "text", "text": t})
		default:
			out = append(out, pm)
		}
	}
	return out
}

func extractEmbeddedFunctionCalls(message *map[string]interface{}) []interface{} {
	content, ok := (*message)["content"].([]interface{})
	if !ok {
		return nil
	}
	var calls []interface{}
	var texts []string
	for _, part := range content {
		pm, ok := part.(map[string]interface{})
		if !ok {
			continue
		}
		typ, _ := pm["type"].(string)
		if typ == "function_call" {
			calls = append(calls, map[string]interface{}{
				"id":   pm["call_id"],
				"type": "function",
				"function": map[string]interface{}{
					"name":      pm["name"],
					"arguments": pm["arguments"],
				},
			})
		} else if typ == "output_text" || typ == "text" || typ == "" {
			if t, ok := pm["text"].(string); ok {
				texts = append(texts, t)
			}
		}
	}
	if len(calls) == 0 {
		return nil
	}
	(*message)["content"] = joinStrings(texts, "")
	return calls
}

func joinStrings(a []string, sep string) string {
	out := ""
	for i, s := range a {
		if i > 0 {
			out += sep
		}
		out += s
	}
	return out
}

// chatReasoningText extracts thinking/reasoning text (DeepSeek
// reasoning_content and friends) so it survives conversion instead of
// being silently dropped when content is empty.
func chatReasoningText(msg map[string]interface{}) string {
	raw, ok := msg["reasoning_content"]
	if !ok || raw == nil {
		return ""
	}
	switch c := raw.(type) {
	case string:
		return c
	case []interface{}:
		var texts []string
		for _, p := range c {
			if pm, ok := p.(map[string]interface{}); ok {
				if t, ok := pm["text"].(string); ok && t != "" {
					texts = append(texts, t)
				}
			}
		}
		return joinStrings(texts, "\n")
	default:
		b, _ := json.Marshal(raw)
		return string(b)
	}
}

func ChatResponseToResponses(chat map[string]interface{}, model string, created *uint64) (map[string]interface{}, error) {
	choices, ok := chat["choices"].([]interface{})
	if !ok {
		return nil, &ResponsesConversionError{Kind: "invalid", Message: "chat response has no choices array"}
	}
	var output []interface{}
	for _, ch := range choices {
		cm, ok := ch.(map[string]interface{})
		if !ok {
			continue
		}
		msg, _ := cm["message"].(map[string]interface{})
		if msg == nil {
			continue
		}
		var parts []interface{}
		if reasoning := chatReasoningText(msg); reasoning != "" {
			parts = append(parts, map[string]interface{}{
				"type": "output_text", "text": reasoning, "annotations": []interface{}{},
			})
		}
		if content := msg["content"]; content != nil && content != "" {
			text := ""
			switch c := content.(type) {
			case string:
				text = c
			default:
				b, _ := json.Marshal(c)
				text = string(b)
			}
			if text != "" {
				parts = append(parts, map[string]interface{}{
					"type": "output_text", "text": text, "annotations": []interface{}{},
				})
			}
		}
		if len(parts) > 0 {
			output = append(output, map[string]interface{}{
				"type": "message", "role": "assistant",
				"content": parts,
				"status":  "completed",
			})
		}
		if tc, ok := msg["tool_calls"].([]interface{}); ok {
			for _, call := range tc {
				m, ok := call.(map[string]interface{})
				if !ok {
					continue
				}
				fun, _ := m["function"].(map[string]interface{})
				if fun == nil {
					fun = map[string]interface{}{}
				}
				output = append(output, map[string]interface{}{
					"type":      "function_call",
					"call_id":   m["id"],
					"name":      fun["name"],
					"arguments": fun["arguments"],
					"status":    "completed",
				})
			}
		}
	}
	if output == nil {
		output = []interface{}{}
	}
	resp := map[string]interface{}{
		"object": "response",
		"model":  model,
		"output": output,
		"status": "completed",
	}
	if id, ok := chat["id"].(string); ok {
		resp["id"] = id
	}
	var ts uint64
	if created != nil {
		ts = *created
	} else if c, ok := chat["created"].(float64); ok {
		ts = uint64(c)
	} else {
		ts = uint64(time.Now().Unix())
	}
	resp["created_at"] = float64(ts)
	if usage, ok := chat["usage"].(map[string]interface{}); ok {
		resp["usage"] = chatUsageToResponsesUsage(usage)
	}
	return resp, nil
}

func chatUsageToResponsesUsage(usage map[string]interface{}) map[string]interface{} {
	var cached interface{}
	if pt, ok := usage["prompt_tokens_details"].(map[string]interface{}); ok {
		cached = pt["cached_tokens"]
	}
	var reasoning interface{}
	if ct, ok := usage["completion_tokens_details"].(map[string]interface{}); ok {
		reasoning = ct["reasoning_tokens"]
	}
	return map[string]interface{}{
		"input_tokens":  usage["prompt_tokens"],
		"output_tokens": usage["completion_tokens"],
		"total_tokens":  usage["total_tokens"],
		"input_tokens_details": map[string]interface{}{
			"cached_tokens": cached,
		},
		"output_tokens_details": map[string]interface{}{
			"reasoning_tokens": reasoning,
		},
	}
}

type ChatToolCallState struct {
	Index       int
	CallID      string
	Name        string
	Arguments   string
	ItemID      string
	OutputIndex int
}

type ChatSseToResponses struct {
	ResponseID         string
	Model              string
	CreatedAt          uint64
	OutputIndex        int
	ResponseStarted    bool
	MessageOpen        bool
	MessageItemID      string
	MessageOutputIndex int
	Text               string
	ToolCalls          []ChatToolCallState
	Usage              interface{}
	FinishReason       string
}

func NewChatSseToResponses(model string) *ChatSseToResponses {
	return &ChatSseToResponses{
		ResponseID: fmt.Sprintf("resp_%d", time.Now().UnixNano()),
		Model:      model,
		CreatedAt:  uint64(time.Now().Unix()),
	}
}

func (c *ChatSseToResponses) PushFrame(data map[string]interface{}) ([]map[string]interface{}, error) {
	var events []map[string]interface{}
	choices, ok := data["choices"].([]interface{})
	if !ok {
		if usage, ok := data["usage"]; ok {
			c.Usage = chatUsageToResponsesUsageGeneric(usage)
		}
		return events, nil
	}
	if len(choices) == 0 {
		if usage, ok := data["usage"]; ok {
			c.Usage = chatUsageToResponsesUsageGeneric(usage)
		}
		return events, nil
	}
	if usage, ok := data["usage"]; ok {
		c.Usage = chatUsageToResponsesUsageGeneric(usage)
	}
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return events, nil
	}
	if reason, ok := choice["finish_reason"].(string); ok && reason != "" {
		c.FinishReason = reason
	}
	delta, ok := choice["delta"].(map[string]interface{})
	if !ok {
		return events, nil
	}
	if reasoning, ok := delta["reasoning_content"].(string); ok && reasoning != "" {
		events = append(events, c.openText()...)
		c.Text += reasoning
		events = append(events, c.emitTextDelta(reasoning))
	}
	if content, ok := delta["content"].(string); ok && content != "" {
		events = append(events, c.openText()...)
		c.Text += content
		events = append(events, c.emitTextDelta(content))
	}
	if tc, ok := delta["tool_calls"].([]interface{}); ok {
		if !c.ResponseStarted {
			c.ResponseStarted = true
			events = append(events, c.emitCreated())
		}
		for _, callRaw := range tc {
			call, ok := callRaw.(map[string]interface{})
			if !ok {
				continue
			}
			idx := 0
			if v, ok := call["index"].(float64); ok {
				idx = int(v)
			}
			var slot *ChatToolCallState
			for i := range c.ToolCalls {
				if c.ToolCalls[i].Index == idx {
					slot = &c.ToolCalls[i]
					break
				}
			}
			if slot == nil {
				newSlot := ChatToolCallState{
					Index:       idx,
					ItemID:      fmt.Sprintf("fc_%d_%d", c.OutputIndex, idx),
					OutputIndex: c.OutputIndex,
				}
				if id, ok := call["id"].(string); ok {
					newSlot.CallID = id
				}
				if fn, ok := call["function"].(map[string]interface{}); ok {
					if n, ok := fn["name"].(string); ok {
						newSlot.Name = n
					}
				}
				c.OutputIndex++
				events = append(events, c.emitFunctionCallAdded(&newSlot))
				c.ToolCalls = append(c.ToolCalls, newSlot)
				slot = &c.ToolCalls[len(c.ToolCalls)-1]
			}
			if fn, ok := call["function"].(map[string]interface{}); ok {
				if args, ok := fn["arguments"].(string); ok && args != "" {
					slot.Arguments += args
					events = append(events, emitArgumentsDelta(slot, args))
				}
			}
		}
	}
	return events, nil
}

func chatUsageToResponsesUsageGeneric(u interface{}) map[string]interface{} {
	if m, ok := u.(map[string]interface{}); ok {
		return chatUsageToResponsesUsage(m)
	}
	return nil
}

func (c *ChatSseToResponses) outputItemStatus() string {
	if c.FinishReason == "length" || c.FinishReason == "content_filter" {
		return "incomplete"
	}
	return "completed"
}

func (c *ChatSseToResponses) Finish() []map[string]interface{} {
	var events []map[string]interface{}
	if c.MessageOpen && c.Text != "" {
		events = append(events, map[string]interface{}{
			"type": "response.output_text.done", "item_id": c.MessageItemID, "output_index": c.MessageOutputIndex, "content_index": 0, "text": c.Text,
		})
		events = append(events, map[string]interface{}{
			"type": "response.content_part.done", "item_id": c.MessageItemID, "output_index": c.MessageOutputIndex, "content_index": 0,
			"part": map[string]interface{}{"type": "output_text", "text": c.Text, "annotations": []interface{}{}},
		})
		events = append(events, map[string]interface{}{
			"type": "response.output_item.done", "output_index": c.MessageOutputIndex, "item": c.messageItem(c.outputItemStatus()),
		})
	}
	for _, call := range c.ToolCalls {
		events = append(events, map[string]interface{}{
			"type": "response.function_call_arguments.done", "item_id": call.ItemID, "output_index": call.OutputIndex, "arguments": call.Arguments,
		})
		events = append(events, map[string]interface{}{
			"type": "response.output_item.done", "output_index": call.OutputIndex, "item": toolCallItem(&call, c.outputItemStatus()),
		})
	}
	status := "completed"
	eventType := "response.completed"
	var incompleteDetails map[string]interface{}
	switch c.FinishReason {
	case "length":
		status = "incomplete"
		eventType = "response.incomplete"
		incompleteDetails = map[string]interface{}{"reason": "max_output_tokens"}
	case "content_filter":
		status = "incomplete"
		eventType = "response.incomplete"
		incompleteDetails = map[string]interface{}{"reason": "content_filter"}
	}
	resp := map[string]interface{}{
		"id": c.ResponseID, "object": "response", "created_at": float64(c.CreatedAt), "status": status, "model": c.Model, "output": c.completedOutput(),
	}
	if incompleteDetails != nil {
		resp["incomplete_details"] = incompleteDetails
	}
	if c.Usage != nil {
		resp["usage"] = c.Usage
	}
	events = append(events, map[string]interface{}{"type": eventType, "response": resp})
	return events
}

func (c *ChatSseToResponses) FailedEvent(msg string) map[string]interface{} {
	return map[string]interface{}{
		"type": "response.failed",
		"response": map[string]interface{}{
			"id": c.ResponseID, "object": "response", "created_at": float64(c.CreatedAt), "status": "failed", "model": c.Model,
			"error": map[string]interface{}{"message": msg, "type": "conversion_error"},
		},
	}
}

func (c *ChatSseToResponses) completedOutput() []interface{} {
	var out []interface{}
	if c.MessageOpen && c.Text != "" {
		out = append(out, c.messageItem(c.outputItemStatus()))
	}
	for _, call := range c.ToolCalls {
		out = append(out, toolCallItem(&call, c.outputItemStatus()))
	}
	if out == nil {
		out = []interface{}{}
	}
	return out
}

func (c *ChatSseToResponses) messageItem(status string) map[string]interface{} {
	return map[string]interface{}{
		"id": c.MessageItemID, "type": "message", "role": "assistant", "status": status,
		"content": []interface{}{map[string]interface{}{"type": "output_text", "text": c.Text, "annotations": []interface{}{}}},
	}
}

func toolCallItem(call *ChatToolCallState, status string) map[string]interface{} {
	return map[string]interface{}{
		"id": call.ItemID, "type": "function_call", "call_id": call.CallID, "name": call.Name, "arguments": call.Arguments, "status": status,
	}
}

func (c *ChatSseToResponses) emitCreated() map[string]interface{} {
	return map[string]interface{}{
		"type":     "response.created",
		"response": map[string]interface{}{"id": c.ResponseID, "object": "response", "created_at": float64(c.CreatedAt), "status": "in_progress", "model": c.Model, "output": []interface{}{}},
	}
}

func (c *ChatSseToResponses) emitMessageAdded() map[string]interface{} {
	c.MessageItemID = fmt.Sprintf("msg_%d", c.OutputIndex)
	c.MessageOutputIndex = c.OutputIndex
	c.OutputIndex++
	return map[string]interface{}{
		"type": "response.output_item.added", "output_index": c.MessageOutputIndex,
		"item": map[string]interface{}{"id": c.MessageItemID, "type": "message", "role": "assistant", "status": "in_progress", "content": []interface{}{}},
	}
}

func (c *ChatSseToResponses) emitContentPartAdded() map[string]interface{} {
	return map[string]interface{}{
		"type": "response.content_part.added", "item_id": c.MessageItemID, "output_index": c.MessageOutputIndex, "content_index": 0,
		"part": map[string]interface{}{"type": "output_text", "text": "", "annotations": []interface{}{}},
	}
}

// openText emits the response/message/part opening events once, shared by
// content and reasoning deltas.
func (c *ChatSseToResponses) openText() []map[string]interface{} {
	var events []map[string]interface{}
	if !c.ResponseStarted {
		c.ResponseStarted = true
		events = append(events, c.emitCreated())
	}
	if !c.MessageOpen {
		c.MessageOpen = true
		events = append(events, c.emitMessageAdded())
		events = append(events, c.emitContentPartAdded())
	}
	return events
}
func (c *ChatSseToResponses) emitTextDelta(delta string) map[string]interface{} {
	return map[string]interface{}{
		"type": "response.output_text.delta", "item_id": c.MessageItemID, "output_index": c.MessageOutputIndex, "content_index": 0, "delta": delta,
	}
}

func (c *ChatSseToResponses) emitFunctionCallAdded(call *ChatToolCallState) map[string]interface{} {
	return map[string]interface{}{
		"type": "response.output_item.added", "output_index": call.OutputIndex,
		"item": map[string]interface{}{"id": call.ItemID, "type": "function_call", "call_id": call.CallID, "name": call.Name, "arguments": "", "status": "in_progress"},
	}
}

func emitArgumentsDelta(call *ChatToolCallState, delta string) map[string]interface{} {
	return map[string]interface{}{
		"type": "response.function_call_arguments.delta", "item_id": call.ItemID, "output_index": call.OutputIndex, "delta": delta,
	}
}
