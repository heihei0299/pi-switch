package inference

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/domain/protocol"
	"github.com/heihei0299/pi-switch/internal/domain/provider"
)

func TestProcessNonStreamResponseConvertsAndCalculatesUsage(t *testing.T) {
	plan, err := protocol.PlanRequest("chat", protocol.OpenAIResponses, "auto")
	if err != nil {
		t.Fatal(err)
	}
	prepared := PreparedRequest{
		Model:        "gpt-4o-mini",
		ProtocolPlan: plan,
		ModelEntry: &provider.ModelEntry{Cost: &provider.ModelCost{
			Input: 3, Output: 15, CacheRead: 0.3,
		}},
	}
	raw := []byte(`{"id":"resp_a","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":10,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`)

	result, err := processNonStreamResponse(prepared, raw)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(result.Body, &body); err != nil {
		t.Fatal(err)
	}
	choices, ok := body["choices"].([]interface{})
	if !ok || len(choices) != 1 {
		t.Fatalf("converted response = %s, want chat choices", result.Body)
	}
	message := choices[0].(map[string]interface{})["message"].(map[string]interface{})
	if message["content"] != "hello" {
		t.Fatalf("converted response content = %v, want hello", message["content"])
	}
	if result.Usage == nil || result.Usage.PromptTokens != 10 || result.Usage.CompletionTokens != 2 {
		t.Fatalf("usage = %+v, want 10 prompt and 2 completion tokens", result.Usage)
	}
	if result.Cost == nil || math.Abs(*result.Cost-0.00006) > 1e-12 {
		t.Fatalf("cost = %v, want 0.00006", result.Cost)
	}
}

func TestProcessNonStreamResponsePassesThroughBytesAndUnknownUsage(t *testing.T) {
	plan, err := protocol.PlanRequest("chat", protocol.OpenAIChat, "auto")
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("not JSON")
	result, err := processNonStreamResponse(PreparedRequest{ProtocolPlan: plan}, raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Body) != string(raw) || result.Usage != nil || result.Cost != nil {
		t.Fatalf("result = %+v, want exact passthrough with unknown usage", result)
	}
}

func TestProcessNonStreamResponseReportsConversionDecodeErrors(t *testing.T) {
	plan, err := protocol.PlanRequest("chat", protocol.OpenAIResponses, "auto")
	if err != nil {
		t.Fatal(err)
	}
	_, err = processNonStreamResponse(PreparedRequest{Model: "m", ProtocolPlan: plan}, []byte("not JSON"))
	if err == nil || !strings.HasPrefix(err.Error(), "upstream response is not valid JSON:") {
		t.Fatalf("conversion error = %v", err)
	}
}
