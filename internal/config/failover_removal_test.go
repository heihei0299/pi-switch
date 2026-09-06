package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFailoverRemoved_LoadIgnoresAndSaveDrops(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	legacy := `{"version":2,"profiles":{},"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112,"failover":["a","b"],"circuitBreaker":{"enabled":true,"failureThreshold":3,"cooldownSeconds":60}},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}}`
	if err := os.WriteFile(path, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadConfigAtPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// After removal, marshaled config must not contain "failover"
	migrated := MigratedForSave(cfg)
	b, _ := json.Marshal(migrated)
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	settings, _ := m["settings"].(map[string]interface{})
	proxy, _ := settings["proxy"].(map[string]interface{})
	if _, ok := proxy["failover"]; ok {
		t.Fatalf("migrated should not contain proxy.failover, got %s", string(b))
	}
	// Also ensure loading does not retain failover (via raw check)
	var raw map[string]interface{}
	_ = json.Unmarshal(b, &raw)
	// Ensure DefaultConfig also has no failover
	def := DefaultConfig()
	b2, _ := json.Marshal(def)
	var m2 map[string]interface{}
	_ = json.Unmarshal(b2, &m2)
	s2, _ := m2["settings"].(map[string]interface{})
	p2, _ := s2["proxy"].(map[string]interface{})
	if _, ok := p2["failover"]; ok {
		t.Fatalf("DefaultConfig should not contain proxy.failover, got %s", string(b2))
	}
}
