package translator

import "testing"

func TestChatToResponses_NormalizesContent(t *testing.T) {
	cases := []struct {
		name string
		body map[string]interface{}
		want []interface{}
	}{
		{
			name: "text array to input_text",
			body: map[string]interface{}{
				"model": "muse",
				"messages": []interface{}{map[string]interface{}{
					"role": "user", "content": []interface{}{map[string]interface{}{"type": "text", "text": "hi"}},
				}},
			},
			want: []interface{}{map[string]interface{}{"role": "user", "content": []interface{}{map[string]interface{}{"type": "input_text", "text": "hi"}}}},
		},
		{
			name: "image_url to input_image",
			body: map[string]interface{}{
				"model": "muse",
				"messages": []interface{}{map[string]interface{}{
					"role": "user", "content": []interface{}{map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "https://example.com/a.png"}}},
				}},
			},
			want: []interface{}{map[string]interface{}{"role": "user", "content": []interface{}{map[string]interface{}{"type": "input_image", "image_url": "https://example.com/a.png"}}}},
		},
		{
			name: "string content wrapped",
			body: map[string]interface{}{
				"model":    "muse",
				"messages": []interface{}{map[string]interface{}{"role": "user", "content": "hi"}},
			},
			want: []interface{}{map[string]interface{}{"role": "user", "content": []interface{}{map[string]interface{}{"type": "input_text", "text": "hi"}}}},
		},
		{
			name: "unknown type passthrough",
			body: map[string]interface{}{
				"model": "muse",
				"messages": []interface{}{map[string]interface{}{
					"role": "user", "content": []interface{}{map[string]interface{}{"type": "custom", "foo": "bar"}},
				}},
			},
			want: []interface{}{map[string]interface{}{"role": "user", "content": []interface{}{map[string]interface{}{"type": "custom", "foo": "bar"}}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := ChatToResponses(tc.body)
			in, _ := out["input"].([]interface{})
			if len(in) != len(tc.want) {
				t.Fatalf("input len %d want %d out=%v", len(in), len(tc.want), out["input"])
			}
			// compare first item content
			gotM, _ := in[0].(map[string]interface{})
			wantM, _ := tc.want[0].(map[string]interface{})
			gotC, _ := gotM["content"].([]interface{})
			wantC, _ := wantM["content"].([]interface{})
			if len(gotC) != len(wantC) {
				t.Fatalf("content len %d want %d got=%v want=%v", len(gotC), len(wantC), gotC, wantC)
			}
			for i := range wantC {
				gm, _ := gotC[i].(map[string]interface{})
				wm, _ := wantC[i].(map[string]interface{})
				for k, wv := range wm {
					if gm[k] != wv {
						t.Fatalf("content[%d] key %s got %v want %v full got=%v", i, k, gm[k], wv, gotC)
					}
				}
			}
		})
	}
}

func TestResponsesToChatConvertsInstructionsToSystemMessage(t *testing.T) {
	out, err := ResponsesToChat(map[string]interface{}{
		"model":        "muse",
		"instructions": "follow this",
		"input":        []interface{}{map[string]interface{}{"role": "user", "content": []interface{}{map[string]interface{}{"type": "input_text", "text": "hello"}}}},
	})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	messages, _ := out["messages"].([]interface{})
	if len(messages) != 2 {
		t.Fatalf("messages = %v", messages)
	}
	system, _ := messages[0].(map[string]interface{})
	if system["role"] != "system" || system["content"] != "follow this" {
		t.Fatalf("system message = %v", system)
	}
}

func TestChatToResponsesCombinesSystemAndDeveloperInstructions(t *testing.T) {
	out := ChatToResponses(map[string]interface{}{
		"model": "muse",
		"messages": []interface{}{
			map[string]interface{}{"role": "system", "content": "system rule"},
			map[string]interface{}{"role": "developer", "content": "developer rule"},
			map[string]interface{}{"role": "user", "content": "hello"},
		},
	})
	if out["instructions"] != "system rule\ndeveloper rule" {
		t.Fatalf("instructions = %v", out["instructions"])
	}
	input, _ := out["input"].([]interface{})
	if len(input) != 1 {
		t.Fatalf("input = %v, want only user message", input)
	}
	message, _ := input[0].(map[string]interface{})
	if message["role"] != "user" {
		t.Fatalf("input role = %v, want user", message["role"])
	}
}
