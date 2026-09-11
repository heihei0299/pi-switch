package translator

import (
	"fmt"

	"github.com/heihei0299/pi-switch/internal/protocol"
)

// Format identifies one side of a protocol conversion, mirroring
// CLIProxyAPI's sdk/translator Format registry in a map-based form.
type Format string

const (
	FormatOpenAIChat      Format = "openai.chat"
	FormatOpenAIResponses Format = "openai.responses"
	FormatAnthropic       Format = "anthropic.messages"
)

// RequestTransform converts an inbound body (already cloned by the caller)
// into the upstream wire format. model is the resolved upstream model id.
type RequestTransform func(model string, body map[string]interface{}) (map[string]interface{}, error)

// ResponseTransform converts a non-streaming upstream JSON body back into
// the downstream (client-facing) protocol. model is the resolved model id.
type ResponseTransform func(upstream map[string]interface{}, model string) (map[string]interface{}, error)

type entry struct {
	request  RequestTransform
	response ResponseTransform
}

var registry = map[Format]map[Format]*entry{}

// Register installs a directional conversion pair. Later registrations win,
// so tests or future providers can override a built-in pair.
func Register(from, to Format, req RequestTransform, resp ResponseTransform) {
	if registry[from] == nil {
		registry[from] = map[Format]*entry{}
	}
	registry[from][to] = &entry{request: req, response: resp}
}

func init() {
	// Responses -> Chat (convert mode against openai-completions upstreams).
	Register(FormatOpenAIResponses, FormatOpenAIChat,
		func(model string, body map[string]interface{}) (map[string]interface{}, error) {
			return ResponsesToChat(body)
		},
		func(upstream map[string]interface{}, model string) (map[string]interface{}, error) {
			return ChatResponseToResponses(upstream, model, nil)
		},
	)
	// Chat -> Responses (chat clients against openai-responses upstreams).
	Register(FormatOpenAIChat, FormatOpenAIResponses,
		func(model string, body map[string]interface{}) (map[string]interface{}, error) {
			return ChatToResponses(body), nil
		},
		func(upstream map[string]interface{}, model string) (map[string]interface{}, error) {
			// Upstream already speaks Responses; nothing to convert back.
			return upstream, nil
		},
	)
	// Chat -> Anthropic.
	Register(FormatOpenAIChat, FormatAnthropic,
		func(model string, body map[string]interface{}) (map[string]interface{}, error) {
			return OpenAIToAnthropic(body), nil
		},
		func(upstream map[string]interface{}, model string) (map[string]interface{}, error) {
			return AnthropicToOpenAIResponse(upstream), nil
		},
	)
	// Anthropic -> Chat.
	Register(FormatAnthropic, FormatOpenAIChat,
		func(model string, body map[string]interface{}) (map[string]interface{}, error) {
			return AnthropicToChat(body), nil
		},
		func(upstream map[string]interface{}, model string) (map[string]interface{}, error) {
			// Upstream already speaks Chat; nothing to convert back.
			return upstream, nil
		},
	)
}

// Plan describes one routed conversion: client-facing From becomes
// upstream wire format To, sent to UpstreamPath.
type Plan struct {
	From         Format
	To           Format
	UpstreamPath string
	// Passthrough is true when no conversion is needed.
	Passthrough bool
	entry       *entry
}

// TransformRequest runs the registered request conversion, or returns body
// unchanged for passthrough plans.
func (p Plan) TransformRequest(model string, body map[string]interface{}) (map[string]interface{}, error) {
	if p.Passthrough || p.entry == nil || p.entry.request == nil {
		return body, nil
	}
	return p.entry.request(model, body)
}

// TransformResponse runs the registered non-streaming response conversion,
// or returns upstream unchanged for passthrough plans.
func (p Plan) TransformResponse(upstream map[string]interface{}, model string) (map[string]interface{}, error) {
	if p.Passthrough || p.entry == nil || p.entry.response == nil {
		return upstream, nil
	}
	return p.entry.response(upstream, model)
}

// NeedsConvert reports whether the request was translated (and the
// non-streaming response therefore needs translation back).
func (p Plan) NeedsConvert() bool { return !p.Passthrough }

func inboundFormat(proto string) (Format, error) {
	switch proto {
	case "responses":
		return FormatOpenAIResponses, nil
	case "messages":
		return FormatAnthropic, nil
	case "chat", "":
		return FormatOpenAIChat, nil
	default:
		return "", fmt.Errorf("unknown protocol %q", proto)
	}
}

func upstreamFormat(api string) (Format, error) {
	if !protocol.CanProxy(api) {
		return "", fmt.Errorf("unsupported api %q", api)
	}
	switch api {
	case protocol.OpenAIResponses:
		return FormatOpenAIResponses, nil
	case protocol.OpenAIChat:
		return FormatOpenAIChat, nil
	case protocol.AnthropicMessages:
		return FormatAnthropic, nil
	default:
		return "", fmt.Errorf("unsupported api %q", api)
	}
}

func upstreamPathFor(to Format) string {
	switch to {
	case FormatOpenAIResponses:
		return "/v1/responses"
	case FormatAnthropic:
		return "/v1/messages"
	default:
		return "/v1/chat/completions"
	}
}

// PlanRequest resolves the conversion plan for one (inbound protocol,
// upstream api, responsesMode) triple. It enforces the same compatibility
// rules as ValidateResponsesMode at request time so misconfigured profiles
// fail with a routable error instead of a mistranslated request.
func PlanRequest(proto, api, mode string) (Plan, error) {
	from, err := inboundFormat(proto)
	if mode == "" {
		mode = "auto"
	}
	if err != nil {
		return Plan{}, err
	}
	to, err := upstreamFormat(api)
	if err != nil {
		return Plan{}, err
	}
	if from == to {
		// Same-format pairs still honor responsesMode: an explicit convert
		// on openai-responses (or passthrough on openai-completions) is a
		// configuration error, not a passthrough.
		if err := ValidateResponsesMode(api, mode); err != nil {
			return Plan{}, err
		}
		if api == protocol.OpenAIResponses && !IsNativeResponsesPassthrough(api, mode) {
			return Plan{}, fmt.Errorf("profile api %s does not support responses with mode %q", api, mode)
		}
		if api == protocol.OpenAIChat && proto == "responses" && !IsChatConvert(api, mode) {
			return Plan{}, fmt.Errorf("profile api %s does not support responses with mode %q", api, mode)
		}
		return Plan{From: from, To: to, UpstreamPath: upstreamPathFor(to), Passthrough: true}, nil
	}
	// Cross-format pairs.
	switch {
	case from == FormatOpenAIResponses && to == FormatOpenAIChat:
		if !IsChatConvert(api, mode) {
			return Plan{}, fmt.Errorf("profile api %s does not support responses with mode %q", api, mode)
		}
	case from == FormatOpenAIChat && to == FormatOpenAIResponses:
		if !IsNativeResponsesPassthrough(api, mode) {
			return Plan{}, fmt.Errorf("profile api %s does not support chat with mode %q", api, mode)
		}
	case from == FormatAnthropic && to == FormatOpenAIChat:
		if api != protocol.OpenAIChat {
			return Plan{}, fmt.Errorf("profile api %s does not support messages", api)
		}
	case from == FormatOpenAIChat && to == FormatAnthropic:
		if api != protocol.AnthropicMessages {
			return Plan{}, fmt.Errorf("profile api %s does not support chat", api)
		}
	default:
		return Plan{}, fmt.Errorf("no translator from %s to %s", from, to)
	}
	sub := registry[from][to]
	if sub == nil {
		return Plan{}, fmt.Errorf("no translator from %s to %s", from, to)
	}
	return Plan{From: from, To: to, UpstreamPath: upstreamPathFor(to), entry: sub}, nil
}

// StreamEventConverter turns one decoded upstream SSE data payload into
// zero or more downstream SSE event payloads. Finish drains terminal
// payloads when the upstream body ends.
type StreamEventConverter interface {
	PushEvent(data map[string]interface{}) ([]map[string]interface{}, error)
	Finish() []map[string]interface{}
	// UsagePayload returns the raw upstream usage payload seen on the stream,
	// or nil when the stream carried none.
	UsagePayload() interface{}
}

// PushEvent adapts ChatSseToResponses to StreamEventConverter.
func (c *ChatSseToResponses) PushEvent(data map[string]interface{}) ([]map[string]interface{}, error) {
	return c.PushFrame(data)
}

// UsagePayload returns the raw upstream usage payload seen on the stream.
func (c *ChatSseToResponses) UsagePayload() interface{} {
	if c == nil {
		return nil
	}
	return c.Usage
}

var streamRegistry = map[Format]map[Format]func(model string) StreamEventConverter{}

// RegisterStream installs a streaming conversion for one direction.
// Later registrations win, mirroring Register.
func RegisterStream(from, to Format, nw func(model string) StreamEventConverter) {
	if streamRegistry[from] == nil {
		streamRegistry[from] = map[Format]func(model string) StreamEventConverter{}
	}
	streamRegistry[from][to] = nw
}

func init() {
	// Upstream Chat SSE -> downstream Responses SSE.
	RegisterStream(FormatOpenAIChat, FormatOpenAIResponses,
		func(model string) StreamEventConverter { return NewChatSseToResponses(model) })
	// Upstream Responses SSE -> downstream Chat SSE.
	RegisterStream(FormatOpenAIResponses, FormatOpenAIChat,
		func(model string) StreamEventConverter { return NewResponsesSseToChat(model) })
}

// StreamConverter returns the streaming converter for this plan's
// upstream-to-downstream direction, or nil when the stream passes through
// unconverted (same-format pairs and directions without a translator).
// model is the downstream (client-facing) model id.
func (p Plan) StreamConverter(model string) StreamEventConverter {
	if p.Passthrough {
		return nil
	}
	// NOTE: the relay converts upstream format back to downstream format,
	// i.e. the reverse of the request direction.
	if nw := streamRegistry[p.To][p.From]; nw != nil {
		return nw(model)
	}
	return nil
}
