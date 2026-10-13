// Package protocol preserves the legacy protocol entrypoints. Rules live in domain/protocol.
package protocol

import domainprotocol "github.com/heihei0299/pi-switch/internal/domain/protocol"

const (
	OpenAIChat         = domainprotocol.OpenAIChat
	OpenAIResponses    = domainprotocol.OpenAIResponses
	AnthropicMessages  = domainprotocol.AnthropicMessages
	GoogleGenerativeAI = domainprotocol.GoogleGenerativeAI

	ResponsesAuto        = domainprotocol.ResponsesAuto
	ResponsesPassthrough = domainprotocol.ResponsesPassthrough
	ResponsesConvert     = domainprotocol.ResponsesConvert
)

type APICapability = domainprotocol.APICapability

func IsKnown(api string) bool                   { return domainprotocol.IsKnown(api) }
func CanProxy(api string) bool                  { return domainprotocol.CanProxy(api) }
func CanGateway(api string) bool                { return domainprotocol.CanGateway(api) }
func AllowedResponsesModes(api string) []string { return domainprotocol.AllowedResponsesModes(api) }
func DefaultResponsesMode(api string) string    { return domainprotocol.DefaultResponsesMode(api) }
func ValidateResponsesMode(api, mode string) error {
	return domainprotocol.ValidateResponsesMode(api, mode)
}
func Capabilities() []APICapability { return domainprotocol.Capabilities() }
