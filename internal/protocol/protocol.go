// Package protocol owns the channel API identifiers and which surfaces support
// each one. It is a leaf package (no internal imports) so config, gateway and
// translator can all agree on one list instead of keeping a copy each.
package protocol

// API identifiers, as they appear in a channel's "api" field.
const (
	OpenAIChat         = "openai-completions"
	OpenAIResponses    = "openai-responses"
	AnthropicMessages  = "anthropic-messages"
	GoogleGenerativeAI = "google-generative-ai"
)

// IsKnown reports whether api is a recognized identifier. This is the set the
// config boundary may store, even when no current surface can route it.
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
