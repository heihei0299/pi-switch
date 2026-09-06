package translator

import (
	"fmt"
	"time"
)

// ResponsesSseToChat converts an upstream Responses SSE stream into
// OpenAI Chat Completions chunks. It mirrors ChatSseToResponses in the
// opposite direction: text deltas become delta.content chunks, function
// call arguments become tool_calls chunks, and the terminal
// response.completed event becomes the finish_reason chunk carrying usage.
type ResponsesSseToChat struct {
	Model     string
	ChatID    string
	CreatedAt uint64
	// Usage holds the raw Responses-format usage map from response.completed,
	// for the same usage.ExtractUsage fallback the server applies elsewhere.
	Usage interface{}

	started bool
	calls   map[string]*responsesToolCall
	order   []string
	failed  string
	done    bool
}

type responsesToolCall struct {
	index      int
	itemID     string
	callID     string
	name       string
	headerSent bool
}

func NewResponsesSseToChat(model string) *ResponsesSseToChat {
	now := time.Now()
	return &ResponsesSseToChat{
		Model:     model,
		ChatID:    fmt.Sprintf("chatcmpl-%d", now.UnixNano()),
		CreatedAt: uint64(now.Unix()),
		calls:     map[string]*responsesToolCall{},
	}
}

func (c *ResponsesSseToChat) chunk(delta map[string]interface{}, finish *string) map[string]interface{} {
	choice := map[string]interface{}{"index": 0, "delta": delta, "finish_reason": nil}
	if finish != nil {
		choice["finish_reason"] = *finish
		choice["delta"] = map[string]interface{}{}
	}
	return map[string]interface{}{
		"id":      c.ChatID,
		"object":  "chat.completion.chunk",
		"created": c.CreatedAt,
		"model":   c.Model,
		"choices": []interface{}{choice},
	}
}

func (c *ResponsesSseToChat) roleChunk() map[string]interface{} {
	c.started = true
	return c.chunk(map[string]interface{}{"role": "assistant"}, nil)
}

func (c *ResponsesSseToChat) ensureStarted() map[string]interface{} {
	if !c.started {
		return c.roleChunk()
	}
	return nil
}

// PushEvent consumes one decoded Responses SSE data payload and returns zero
// or more Chat chunks. Unknown event types are ignored.
func (c *ResponsesSseToChat) PushEvent(data map[string]interface{}) ([]map[string]interface{}, error) {
	if c.done {
		return nil, nil
	}
	typ, _ := data["type"].(string)
	var out []map[string]interface{}
	emit := func(ch map[string]interface{}) {
		if ch != nil {
			out = append(out, ch)
		}
	}
	switch typ {
	case "response.created":
		emit(c.roleChunk())
	case "response.output_text.delta":
		emit(c.ensureStarted())
		if delta, ok := data["delta"].(string); ok && delta != "" {
			emit(c.chunk(map[string]interface{}{"content": delta}, nil))
		}
	case "response.output_item.added":
		item, _ := data["item"].(map[string]interface{})
		if item == nil {
			break
		}
		if item["type"] != "function_call" {
			emit(c.ensureStarted())
			break
		}
		itemID, _ := item["id"].(string)
		if itemID == "" {
			if id, ok := item["call_id"].(string); ok {
				itemID = id
			}
		}
		if itemID == "" {
			break
		}
		if old, dup := c.calls[itemID]; dup {
			// A late added after synthesized state only fills blanks.
			if old.name == "" {
				old.name, _ = item["name"].(string)
			}
			if old.callID == "" {
				old.callID, _ = item["call_id"].(string)
			}
			break
		}
		callID, _ := item["call_id"].(string)
		name, _ := item["name"].(string)
		st := &responsesToolCall{
			index:  len(c.order),
			itemID: itemID,
			callID: callID,
			name:   name,
		}
		c.calls[itemID] = st
		c.order = append(c.order, itemID)
		emit(c.ensureStarted())
		emit(c.toolHeaderChunk(st, ""))
		st.headerSent = true
	case "response.function_call_arguments.delta":
		itemID, _ := data["item_id"].(string)
		st := c.calls[itemID]
		if st == nil {
			// Upstream skipped output_item.added: synthesize state so the
			// payload survives. The header is suppressed until a name (or a
			// late added) arrives; deltas stream headerless.
			st = &responsesToolCall{index: len(c.order), itemID: itemID}
			c.calls[itemID] = st
			c.order = append(c.order, itemID)
			st.headerSent = true
		}
		delta, _ := data["delta"].(string)
		emit(c.ensureStarted())
		if !st.headerSent {
			emit(c.toolHeaderChunk(st, ""))
			st.headerSent = true
		}
		if delta != "" {
			emit(c.toolDeltaChunk(st, delta))
		}
	case "response.completed":
		if resp, ok := data["response"].(map[string]interface{}); ok {
			if u, ok := resp["usage"]; ok {
				c.Usage = u
			}
		}
		c.done = true
		out = append(out, c.finishChunk(c.finishReason()))
	case "response.failed":
		if resp, ok := data["response"].(map[string]interface{}); ok {
			if em, ok := resp["error"].(map[string]interface{}); ok {
				c.failed, _ = em["message"].(string)
			}
		}
		c.done = true
		out = append(out, c.finishChunk("stop"))
	}
	return out, nil
}

func (c *ResponsesSseToChat) toolHeaderChunk(st *responsesToolCall, args string) map[string]interface{} {
	return c.chunk(map[string]interface{}{
		"tool_calls": []interface{}{map[string]interface{}{
			"index": st.index,
			"id":    st.callID,
			"type":  "function",
			"function": map[string]interface{}{
				"name":      st.name,
				"arguments": args,
			},
		}},
	}, nil)
}

func (c *ResponsesSseToChat) toolDeltaChunk(st *responsesToolCall, delta string) map[string]interface{} {
	return c.chunk(map[string]interface{}{
		"tool_calls": []interface{}{map[string]interface{}{
			"index": st.index,
			"function": map[string]interface{}{
				"arguments": delta,
			},
		}},
	}, nil)
}

func (c *ResponsesSseToChat) finishReason() string {
	if len(c.order) > 0 {
		return "tool_calls"
	}
	return "stop"
}

// finishChunk emits the terminal Chat chunk with the given finish_reason and
// converted usage. Responses usage maps to Chat usage inline (input->prompt,
// output->completion); richer detail parsing stays in usage.ExtractUsage via
// the Usage field fallback in the server relay.
func (c *ResponsesSseToChat) finishChunk(reason string) map[string]interface{} {
	ch := c.chunk(map[string]interface{}{}, &reason)
	if m, ok := c.Usage.(map[string]interface{}); ok {
		num := func(k string) interface{} {
			if v, ok := m[k]; ok {
				return v
			}
			return float64(0)
		}
		prompt := num("input_tokens")
		completion := num("output_tokens")
		total := num("total_tokens")
		if total == float64(0) {
			if pf, ok := prompt.(float64); ok {
				if cf, ok := completion.(float64); ok {
					total = float64(pf + cf)
				}
			}
		}
		ch["usage"] = map[string]interface{}{
			"prompt_tokens":     prompt,
			"completion_tokens": completion,
			"total_tokens":      total,
		}
	}
	return ch
}

// Finish drains terminal chunks. Responses streams always terminate via
// response.completed/failed, so this is normally empty; it only emits a stop
// chunk when the upstream closed the body without a terminal event.
func (c *ResponsesSseToChat) Finish() []map[string]interface{} {
	if c.done {
		return nil
	}
	c.done = true
	return []map[string]interface{}{c.finishChunk("stop")}
}

// UsagePayload returns the raw Responses usage payload from response.completed.
func (c *ResponsesSseToChat) UsagePayload() interface{} {
	if c == nil {
		return nil
	}
	return c.Usage
}
