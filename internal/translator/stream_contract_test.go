package translator

import "testing"

func TestChatSseToResponses_LengthFinishIsIncomplete(t *testing.T) {
	c := NewChatSseToResponses("m")
	if _, err := c.PushFrame(map[string]interface{}{
		"choices": []interface{}{map[string]interface{}{
			"delta": map[string]interface{}{"content": "partial"},
		}},
	}); err != nil {
		t.Fatalf("content frame: %v", err)
	}
	if _, err := c.PushFrame(map[string]interface{}{
		"choices": []interface{}{map[string]interface{}{
			"delta":         map[string]interface{}{},
			"finish_reason": "length",
		}},
	}); err != nil {
		t.Fatalf("finish frame: %v", err)
	}

	var terminal map[string]interface{}
	for _, event := range c.Finish() {
		if event["type"] == "response.incomplete" {
			terminal = event
		}
	}
	if terminal == nil {
		t.Fatal("length finish must emit response.incomplete")
	}
	response, _ := terminal["response"].(map[string]interface{})
	if response["status"] != "incomplete" {
		t.Fatalf("status = %v, want incomplete", response["status"])
	}
	details, _ := response["incomplete_details"].(map[string]interface{})
	if details["reason"] != "max_output_tokens" {
		t.Fatalf("incomplete reason = %v, want max_output_tokens", details["reason"])
	}
}
