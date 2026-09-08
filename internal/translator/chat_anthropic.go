package translator

import (
	"fmt"
	"strings"
)

// ChatToResponses converts an OpenAI Chat Completions body into an OpenAI
// Responses body. Moved from internal/server (chatToResponses) so every
// conversion lives behind the translator registry.
func ChatToResponses(body map[string]interface{}) map[string]interface{} {
	var input []interface{}
	var instructions []string
	if msgs, ok := body["messages"].([]interface{}); ok {
		for _, m := range msgs {
			if pm, ok := m.(map[string]interface{}); ok {
				role, _ := pm["role"].(string)
				content := normalizeChatContent(pm["content"])
				if role == "system" || role == "developer" {
					if text := chatInstructionText(pm["content"]); text != "" {
						instructions = append(instructions, text)
					}
					continue
				}
				input = append(input, map[string]interface{}{"role": role, "content": content})
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
	for _, k := range []string{"temperature", "top_p", "stream", "stop"} {
		if v, ok := body[k]; ok {
			out[k] = v
		}
	}
	if len(instructions) > 0 {
		out["instructions"] = strings.Join(instructions, "\n")
	}
	if tools, ok := body["tools"]; ok {
		out["tools"] = tools
	}
	return out
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
	model, _ := body["model"].(string)
	anthMsgs, _ := body["messages"].([]interface{})
	var chatMsgs []interface{}
	if system, ok := body["system"]; ok {
		var text string
		switch v := system.(type) {
		case string:
			text = v
		case []interface{}:
			for _, p := range v {
				if pm, ok := p.(map[string]interface{}); ok {
					if t, ok := pm["text"].(string); ok {
						if text != "" {
							text += "\n"
						}
						text += t
					}
				}
			}
		}
		if text != "" {
			chatMsgs = append(chatMsgs, map[string]interface{}{"role": "system", "content": text})
		}
	}
	for _, m := range anthMsgs {
		if pm, ok := m.(map[string]interface{}); ok {
			role, _ := pm["role"].(string)
			content := pm["content"]
			var text string
			switch c := content.(type) {
			case string:
				text = c
			case []interface{}:
				for _, part := range c {
					if pmm, ok := part.(map[string]interface{}); ok {
						if t, ok := pmm["text"].(string); ok {
							if text != "" {
								text += "\n"
							}
							text += t
						}
					}
				}
			default:
				text = fmt.Sprintf("%v", content)
			}
			chatMsgs = append(chatMsgs, map[string]interface{}{"role": role, "content": text})
		}
	}
	out := map[string]interface{}{
		"model":    model,
		"messages": chatMsgs,
	}
	if v, ok := body["max_tokens"]; ok {
		out["max_tokens"] = v
	}
	if v, ok := body["temperature"]; ok {
		out["temperature"] = v
	}
	if v, ok := body["stream"]; ok {
		out["stream"] = v
	}
	if v, ok := body["stop_sequences"]; ok {
		out["stop"] = v
	}
	if chatMsgs == nil {
		out["messages"] = []interface{}{}
	}
	return out
}
