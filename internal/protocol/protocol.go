// Package protocol owns the channel API identifiers and which surfaces support
// each one. It is a leaf package (no internal imports) so config, gateway and
// translator can all agree on one list instead of keeping a copy each.
package protocol

import "fmt"

// API identifiers, as they appear in a channel's "api" field.
const (
	OpenAIChat         = "openai-completions"
	OpenAIResponses    = "openai-responses"
	AnthropicMessages  = "anthropic-messages"
	GoogleGenerativeAI = "google-generative-ai"
)

// IsKnown reports whether api is a recognized identifier.
// Parsing/loading may preserve a known identifier, while authoring/write
// boundaries may additionally require CanProxy.
func IsKnown(api string) bool {
	switch api {
	case OpenAIChat, OpenAIResponses, AnthropicMessages, GoogleGenerativeAI:
		return true
	default:
		return false
	}
}

// CanProxy reports whether the translator can route an upstream with this api.
func CanProxy(api string) bool {
	switch api {
	case OpenAIChat, OpenAIResponses, AnthropicMessages:
		return true
	default:
		return false
	}
}

// CanGateway reports whether the gateway can publish models exposed through this
// api into one of its two fixed providers.
func CanGateway(api string) bool {
	switch api {
	case OpenAIChat, OpenAIResponses:
		return true
	default:
		return false
	}
}

// responsesMode values. "auto" defers to the api; the other two are explicit.
const (
	ResponsesAuto        = "auto"
	ResponsesPassthrough = "passthrough"
	ResponsesConvert     = "convert"
)

// AllowedResponsesModes returns the responsesMode values this api accepts. The
// list is the rule: ValidateResponsesMode below is derived from it.
func AllowedResponsesModes(api string) []string {
	switch api {
	case OpenAIResponses:
		return []string{ResponsesAuto, ResponsesPassthrough}
	case OpenAIChat:
		return []string{ResponsesAuto, ResponsesConvert}
	default:
		return []string{ResponsesAuto}
	}
}

// DefaultResponsesMode is the effective mode an explicit "auto" resolves to for
// this api (the value the UI shows as the derived mode).
func DefaultResponsesMode(api string) string {
	switch api {
	case OpenAIResponses:
		return ResponsesPassthrough
	case OpenAIChat:
		return ResponsesConvert
	default:
		return ResponsesAuto
	}
}

// ValidateResponsesMode is the single responsesMode compatibility rule shared by
// config, profile and translator. An empty mode is the "auto" default.
func ValidateResponsesMode(api, mode string) error {
	if mode == "" {
		mode = ResponsesAuto
	}
	for _, allowed := range AllowedResponsesModes(api) {
		if mode == allowed {
			return nil
		}
	}
	switch mode {
	case ResponsesPassthrough:
		return fmt.Errorf("responsesMode passthrough requires api %s, got %s", OpenAIResponses, api)
	case ResponsesConvert:
		return fmt.Errorf("responsesMode convert requires api %s, got %s", OpenAIChat, api)
	default:
		return fmt.Errorf("invalid responsesMode %q", mode)
	}
}

// APICapability is one api's identity plus the capabilities every surface shares.
// It is what GET /api/state exposes so the WebUI does not keep its own rule set.
type APICapability struct {
	ID             string   `json:"id"`
	Label          string   `json:"label"`
	DefaultMode    string   `json:"defaultMode"`
	ResponsesModes []string `json:"responsesModes"`
	CanProxy       bool     `json:"canProxy"`
	CanGateway     bool     `json:"canGateway"`
}

// Capabilities returns the known apis in presentation order with their
// capabilities derived from the same functions the Go surfaces use.
func Capabilities() []APICapability {
	specs := []struct{ id, label string }{
		{OpenAIChat, "OpenAI Chat Completions"},
		{OpenAIResponses, "OpenAI Responses"},
		{AnthropicMessages, "Anthropic Messages"},
		{GoogleGenerativeAI, "Google Gemini"},
	}
	out := make([]APICapability, 0, len(specs))
	for _, s := range specs {
		out = append(out, APICapability{
			ID:             s.id,
			Label:          s.label,
			DefaultMode:    DefaultResponsesMode(s.id),
			ResponsesModes: AllowedResponsesModes(s.id),
			CanProxy:       CanProxy(s.id),
			CanGateway:     CanGateway(s.id),
		})
	}
	return out
}
