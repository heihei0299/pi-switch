package translator

import "testing"

// RED: registry must resolve the OpenAI bidirectional + Anthropic pairs
// that handleChatCompletions/handleStream currently hard-code in switch statements.
// Contract: docs/system-contract.md §2.4 and the IMP-04 matrix define the supported inbound/provider/mode combinations.
func TestRegistry_PlanRequestPairs(t *testing.T) {
	cases := []struct {
		name     string
		proto    string
		api      string
		mode     string
		wantFrom Format
		wantTo   Format
		wantPath string
	}{
		{"responses passthrough", "responses", "openai-responses", "auto", FormatOpenAIResponses, FormatOpenAIResponses, "/v1/responses"},
		{"responses passthrough explicit", "responses", "openai-responses", "passthrough", FormatOpenAIResponses, FormatOpenAIResponses, "/v1/responses"},
		{"responses convert explicit", "responses", "openai-completions", "convert", FormatOpenAIResponses, FormatOpenAIChat, "/v1/chat/completions"},
		{"responses convert", "responses", "openai-completions", "auto", FormatOpenAIResponses, FormatOpenAIChat, "/v1/chat/completions"},
		{"chat to responses", "chat", "openai-responses", "auto", FormatOpenAIChat, FormatOpenAIResponses, "/v1/responses"},
		{"chat to responses explicit", "chat", "openai-responses", "passthrough", FormatOpenAIChat, FormatOpenAIResponses, "/v1/responses"},
		{"chat passthrough", "chat", "openai-completions", "auto", FormatOpenAIChat, FormatOpenAIChat, "/v1/chat/completions"},
		{"chat convert explicit", "chat", "openai-completions", "convert", FormatOpenAIChat, FormatOpenAIChat, "/v1/chat/completions"},
		{"chat to anthropic", "chat", "anthropic-messages", "auto", FormatOpenAIChat, FormatAnthropic, "/v1/messages"},
		{"messages passthrough", "messages", "anthropic-messages", "auto", FormatAnthropic, FormatAnthropic, "/v1/messages"},
		{"messages to chat", "messages", "openai-completions", "auto", FormatAnthropic, FormatOpenAIChat, "/v1/chat/completions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := PlanRequest(tc.proto, tc.api, tc.mode)
			if err != nil {
				t.Fatalf("PlanRequest error: %v", err)
			}
			if plan.From != tc.wantFrom || plan.To != tc.wantTo || plan.UpstreamPath != tc.wantPath {
				t.Fatalf("got %+v want from=%s to=%s path=%s", plan, tc.wantFrom, tc.wantTo, tc.wantPath)
			}
		})
	}
}

func TestRegistry_EmptyResponsesModeDefaultsToAuto(t *testing.T) {
	for _, tc := range []struct {
		proto string
		api   string
	}{
		{proto: "responses", api: "openai-responses"},
		{proto: "chat", api: "openai-completions"},
	} {
		t.Run(tc.proto+"/"+tc.api, func(t *testing.T) {
			if _, err := PlanRequest(tc.proto, tc.api, ""); err != nil {
				t.Fatalf("PlanRequest with omitted responsesMode: %v", err)
			}
		})
	}
}

func TestRegistry_UnsupportedCombo(t *testing.T) {
	if _, err := PlanRequest("responses", "anthropic-messages", "auto"); err == nil {
		t.Fatal("responses on anthropic-messages must be unsupported")
	}
	if _, err := PlanRequest("responses", "openai-responses", "convert"); err == nil {
		t.Fatal("convert mode on openai-responses must be rejected")
	}
}

func TestRegistry_RequestTransformRoundTrip(t *testing.T) {
	// responses -> chat must carry input string as user message
	plan, err := PlanRequest("responses", "openai-completions", "auto")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	out, err := plan.TransformRequest("gpt-4o-mini", map[string]interface{}{"model": "gpt-4o-mini", "input": "hi"})
	if err != nil {
		t.Fatalf("transform: %v", err)
	}
	msgs, _ := out["messages"].([]interface{})
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %v", out)
	}
	// chat -> responses must carry messages as input (system becomes instructions)
	plan2, err := PlanRequest("chat", "openai-responses", "auto")
	if err != nil {
		t.Fatalf("plan2: %v", err)
	}
	out2, err := plan2.TransformRequest("gpt-4o-mini", map[string]interface{}{
		"model":    "gpt-4o-mini",
		"messages": []interface{}{map[string]interface{}{"role": "system", "content": "sys"}, map[string]interface{}{"role": "user", "content": "hi"}},
	})
	if err != nil {
		t.Fatalf("transform2: %v", err)
	}
	if out2["instructions"] != "sys" {
		t.Fatalf("system should become instructions, got %v", out2)
	}
}

func TestRegistry_ResponseTransformsReturnClientProtocol(t *testing.T) {
	chat := map[string]interface{}{
		"id": "chatcmpl-1", "model": "m", "choices": []interface{}{map[string]interface{}{
			"message": map[string]interface{}{"role": "assistant", "content": "hello"},
			"finish_reason": "stop",
		}},
	}

	chatToResponses, err := PlanRequest("chat", "openai-responses", "auto")
	if err != nil {
		t.Fatal(err)
	}
	responsesUpstream := map[string]interface{}{
		"id": "resp-1", "model": "m", "output": []interface{}{map[string]interface{}{
			"type": "message", "role": "assistant", "content": []interface{}{map[string]interface{}{"type": "output_text", "text": "hello"}},
		}},
	}
	chatResponse, err := chatToResponses.TransformResponse(responsesUpstream, "m")
	if err != nil {
		t.Fatalf("responses to chat: %v", err)
	}
	if chatResponse["object"] != "chat.completion" {
		t.Fatalf("response object = %v, want chat.completion", chatResponse["object"])
	}

	anthropicToChat, err := PlanRequest("messages", "openai-completions", "auto")
	if err != nil {
		t.Fatal(err)
	}
	anthropicResponse, err := anthropicToChat.TransformResponse(chat, "m")
	if err != nil {
		t.Fatalf("chat to anthropic: %v", err)
	}
	if anthropicResponse["type"] != "message" || anthropicResponse["role"] != "assistant" {
		t.Fatalf("response = %v, want Anthropic message", anthropicResponse)
	}
}

func TestRegistry_MissingStreamConverterIsNotPassthrough(t *testing.T) {
	for _, tc := range []struct {
		name  string
		proto string
		api   string
	}{
		{name: "anthropic to chat", proto: "messages", api: "openai-completions"},
		{name: "chat to anthropic", proto: "chat", api: "anthropic-messages"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := PlanRequest(tc.proto, tc.api, "auto")
			if err != nil {
				t.Fatal(err)
			}
			if plan.Passthrough {
				t.Fatal("converted pair must not be marked passthrough")
			}
			if got := plan.StreamConverter("m"); got != nil {
				t.Fatal("unsupported streaming pair unexpectedly has a converter")
			}
		})
	}
}
