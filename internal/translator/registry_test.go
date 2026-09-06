package translator

import "testing"

// RED: registry must resolve the OpenAI bidirectional + Anthropic pairs
// that handleChatCompletions/handleStream currently hard-code in switch statements.
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
		{"responses convert", "responses", "openai-completions", "auto", FormatOpenAIResponses, FormatOpenAIChat, "/v1/chat/completions"},
		{"chat to responses", "chat", "openai-responses", "auto", FormatOpenAIChat, FormatOpenAIResponses, "/v1/responses"},
		{"chat passthrough", "chat", "openai-completions", "auto", FormatOpenAIChat, FormatOpenAIChat, "/v1/chat/completions"},
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
