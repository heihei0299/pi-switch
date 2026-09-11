package protocol

import "testing"

// The capability split is the contract the three surfaces depend on:
// google-generative-ai is a known api the config may store, but neither the
// translator nor the gateway can serve it.
func TestCapabilities(t *testing.T) {
	cases := []struct {
		api                   string
		known, proxy, gateway bool
	}{
		{OpenAIChat, true, true, true},
		{OpenAIResponses, true, true, true},
		{AnthropicMessages, true, true, false},
		{GoogleGenerativeAI, true, false, false},
		{"bogus", false, false, false},
	}
	for _, tc := range cases {
		if got := IsKnown(tc.api); got != tc.known {
			t.Errorf("IsKnown(%q) = %v, want %v", tc.api, got, tc.known)
		}
		if got := CanProxy(tc.api); got != tc.proxy {
			t.Errorf("CanProxy(%q) = %v, want %v", tc.api, got, tc.proxy)
		}
		if got := CanGateway(tc.api); got != tc.gateway {
			t.Errorf("CanGateway(%q) = %v, want %v", tc.api, got, tc.gateway)
		}
	}
}
