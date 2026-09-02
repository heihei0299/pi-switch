package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveMigration_RemovesLegacyAndBumpsVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Write a legacy v1 file with injectOpenCodeAttribution true
	legacy := `{"version":1,"profiles":{},"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"injectOpenCodeAttribution":true}}`
	if err := os.WriteFile(path, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadConfigAtPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Version != 2 {
		t.Fatalf("loaded version = %d, want 2", cfg.Version)
	}
	if cfg.Settings.ConversationSource != "proxy" {
		t.Fatalf("loaded source = %q, want proxy", cfg.Settings.ConversationSource)
	}
	// Simulate save: use helper that should be used by server
	migrated := MigratedForSave(cfg)
	if migrated.Version != 2 {
		t.Fatalf("migrated version = %d, want 2", migrated.Version)
	}
	b, _ := json.Marshal(migrated)
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	settings, _ := m["settings"].(map[string]interface{})
	if _, ok := settings["injectOpenCodeAttribution"]; ok {
		t.Fatalf("migrated should not contain injectOpenCodeAttribution, got %s", string(b))
	}
	if settings["conversationSource"] != "proxy" {
		t.Fatalf("migrated conversationSource = %v, want proxy", settings["conversationSource"])
	}
	// Also test that saving a config constructed with version 1 and sessionScan + legacy via direct struct (simulating old in-memory) gets migrated
	cfg2 := DefaultConfig()
	cfg2.Version = 1
	// Simulate legacy via field? Since we don't have field, we test that MigratedForSave bumps version even without legacy
	if cfg2.Version != 1 {
		t.Fatalf("setup")
	}
	migrated2 := MigratedForSave(cfg2)
	if migrated2.Version != 2 {
		t.Fatalf("bump version = %d, want 2", migrated2.Version)
	}
}

func TestConfigVersion_MigratesAndDefaults(t *testing.T) {
	jsonV1 := `{"version":1,"settings":{"injectOpenCodeAttribution":true}}`
	var cfg PiSwitchConfig
	if err := json.Unmarshal([]byte(jsonV1), &cfg); err != nil {
		// PiSwitchConfig unmarshal may not handle legacy directly, use LoadConfigAtPath instead
		t.Logf("direct unmarshal not supported, using Load")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(jsonV1), 0644)
	loaded, _, _ := LoadConfigAtPath(path)
	if loaded.Version != 2 {
		t.Fatalf("version = %d, want 2", loaded.Version)
	}
	if loaded.Settings.ConversationSource != "proxy" {
		t.Fatalf("source = %q, want proxy", loaded.Settings.ConversationSource)
	}
	jsonDefault := `{"version":1,"settings":{}}`
	os.WriteFile(path, []byte(jsonDefault), 0644)
	loaded2, _, _ := LoadConfigAtPath(path)
	if loaded2.Settings.ConversationSource != "sessionScan" {
		t.Fatalf("default source = %q, want sessionScan", loaded2.Settings.ConversationSource)
	}
	def := DefaultConfig()
	if def.Version != 2 {
		t.Fatalf("default version = %d, want 2", def.Version)
	}
	if def.Settings.ConversationSource != "sessionScan" {
		t.Fatalf("default source = %q, want sessionScan", def.Settings.ConversationSource)
	}
}
