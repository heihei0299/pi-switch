package config

import (
	"encoding/json"
	"testing"
)

func TestSettings_DirectDefaultsToSessionScanWhenMissing(t *testing.T) {
	var s Settings
	if err := json.Unmarshal([]byte(`{}`), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.ConversationSource != "sessionScan" {
		t.Fatalf("ConversationSource = %q, want sessionScan", s.ConversationSource)
	}
}

func TestSettings_DirectParsesAllThreeVariants(t *testing.T) {
	cases := []struct {
		json string
		want string
	}{
		{`{"conversationSource":"proxy"}`, "proxy"},
		{`{"conversationSource":"off"}`, "off"},
		{`{"conversationSource":"sessionScan"}`, "sessionScan"},
	}
	for _, c := range cases {
		var s Settings
		if err := json.Unmarshal([]byte(c.json), &s); err != nil {
			t.Fatalf("unmarshal %s: %v", c.json, err)
		}
		if s.ConversationSource != c.want {
			t.Fatalf("json %s => %q, want %q", c.json, s.ConversationSource, c.want)
		}
		if back, _ := json.Marshal(s); stringContains(string(back), `"conversationSource"`) == false {
			t.Fatalf("marshal should contain conversationSource, got %s", string(back))
		}
	}
}

func stringContains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}

func TestSettings_DirectMigratesLegacyTrueToProxy(t *testing.T) {
	var s Settings
	if err := json.Unmarshal([]byte(`{"injectOpenCodeAttribution":true}`), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.ConversationSource != "proxy" {
		t.Fatalf("legacy true => %q, want proxy", s.ConversationSource)
	}
}

func TestSettings_DirectMigratesLegacyFalseToOff(t *testing.T) {
	var s Settings
	if err := json.Unmarshal([]byte(`{"injectOpenCodeAttribution":false}`), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.ConversationSource != "off" {
		t.Fatalf("legacy false => %q, want off", s.ConversationSource)
	}
}

func TestSettings_DirectExplicitWinsOverLegacy(t *testing.T) {
	var s Settings
	if err := json.Unmarshal([]byte(`{"conversationSource":"off","injectOpenCodeAttribution":true}`), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.ConversationSource != "off" {
		t.Fatalf("explicit off + legacy true => %q, want off", s.ConversationSource)
	}
	b, _ := json.Marshal(s)
	if stringContains(string(b), "injectOpenCodeAttribution") {
		t.Fatalf("marshal should not contain injectOpenCodeAttribution, got %s", string(b))
	}
}
