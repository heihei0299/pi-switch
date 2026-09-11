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

// The responsesMode rule is one function; these are the cases every surface
// (config/profile/translator) now shares.
func TestValidateResponsesMode(t *testing.T) {
	cases := []struct {
		api, mode string
		ok        bool
	}{
		{OpenAIChat, "", true},
		{OpenAIChat, ResponsesAuto, true},
		{OpenAIChat, ResponsesConvert, true},
		{OpenAIChat, ResponsesPassthrough, false},
		{OpenAIResponses, "", true},
		{OpenAIResponses, ResponsesAuto, true},
		{OpenAIResponses, ResponsesPassthrough, true},
		{OpenAIResponses, ResponsesConvert, false},
		{AnthropicMessages, ResponsesAuto, true},
		{AnthropicMessages, ResponsesPassthrough, false},
		{GoogleGenerativeAI, ResponsesConvert, false},
		{OpenAIChat, "bogus", false},
	}
	for _, tc := range cases {
		if err := ValidateResponsesMode(tc.api, tc.mode); (err == nil) != tc.ok {
			t.Errorf("ValidateResponsesMode(%q, %q) err=%v, want ok=%v", tc.api, tc.mode, err, tc.ok)
		}
	}
}

func TestCapabilitiesCoverKnownApis(t *testing.T) {
	caps := Capabilities()
	if len(caps) != 4 {
		t.Fatalf("Capabilities len = %d, want 4", len(caps))
	}
	byID := map[string]APICapability{}
	for _, c := range caps {
		if !IsKnown(c.ID) {
			t.Errorf("capability %q is not IsKnown", c.ID)
		}
		if c.DefaultMode == "" || len(c.ResponsesModes) == 0 {
			t.Errorf("capability %q missing mode data: %+v", c.ID, c)
		}
		byID[c.ID] = c
	}
	if byID[GoogleGenerativeAI].CanProxy || byID[GoogleGenerativeAI].CanGateway {
		t.Errorf("google must be known but not proxiable/gateway: %+v", byID[GoogleGenerativeAI])
	}
	if !byID[AnthropicMessages].CanProxy || byID[AnthropicMessages].CanGateway {
		t.Errorf("anthropic must be proxiable but not gateway: %+v", byID[AnthropicMessages])
	}
}
