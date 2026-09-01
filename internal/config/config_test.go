package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Settings.ProviderPrefix != "pi-switch" {
		t.Fatalf("providerPrefix = %q, want pi-switch", cfg.Settings.ProviderPrefix)
	}
	if cfg.Settings.WriteMode != "gateway" {
		t.Fatalf("writeMode = %q, want gateway", cfg.Settings.WriteMode)
	}
	if cfg.Settings.GatewayAPI != "openai-completions" {
		t.Fatalf("gatewayApi = %q, want openai-completions", cfg.Settings.GatewayAPI)
	}
	if cfg.Settings.Proxy.Host != "127.0.0.1" || cfg.Settings.Proxy.Port != 43112 {
		t.Fatalf("proxy = %v, want 127.0.0.1:43112", cfg.Settings.Proxy)
	}
	if cfg.Settings.Web.Host != "127.0.0.1" || cfg.Settings.Web.Port != 43110 {
		t.Fatalf("web = %v, want 127.0.0.1:43110", cfg.Settings.Web)
	}
	if cfg.Settings.ConversationSource != "sessionScan" {
		t.Fatalf("conversationSource = %q, want sessionScan", cfg.Settings.ConversationSource)
	}
	if cfg.Version != 2 {
		t.Fatalf("version = %d, want 2", cfg.Version)
	}
}

func TestLoadPerRequest_MissingFileReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg, src, err := LoadConfigAtPath(path)
	if err != nil {
		t.Fatalf("LoadConfigAtPath error: %v", err)
	}
	if src != "default (no file)" {
		t.Fatalf("src = %q, want default (no file)", src)
	}
	if cfg.Settings.ProviderPrefix != "pi-switch" {
		t.Fatalf("want default, got %q", cfg.Settings.ProviderPrefix)
	}
}

func TestLoadPerRequest_MigratesLegacyField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// version 1 with injectOpenCodeAttribution true -> should migrate to proxy and bump to 2
	legacy := `{"version":1,"profiles":{},"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"injectOpenCodeAttribution":true}}`
	if err := os.WriteFile(path, []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadConfigAtPath(path)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}
	if cfg.Version != 2 {
		t.Fatalf("version = %d, want 2", cfg.Version)
	}
	if cfg.Settings.ConversationSource != "proxy" {
		t.Fatalf("conversationSource = %q, want proxy", cfg.Settings.ConversationSource)
	}
	// false -> off
	legacy2 := `{"version":1,"profiles":{},"settings":{"injectOpenCodeAttribution":false}}`
	_ = os.WriteFile(path, []byte(legacy2), 0644)
	cfg2, _, _ := LoadConfigAtPath(path)
	if cfg2.Settings.ConversationSource != "off" {
		t.Fatalf("conversationSource = %q, want off", cfg2.Settings.ConversationSource)
	}
}

func TestLoadPerRequest_HotReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// first write
	_ = os.WriteFile(path, []byte(`{"version":2,"profiles":{},"settings":{"providerPrefix":"first"}}`), 0644)
	cfg1, _, _ := LoadConfigAtPath(path)
	if cfg1.Settings.ProviderPrefix != "first" {
		t.Fatalf("first = %q", cfg1.Settings.ProviderPrefix)
	}
	// overwrite
	_ = os.WriteFile(path, []byte(`{"version":2,"profiles":{},"settings":{"providerPrefix":"second"}}`), 0644)
	cfg2, _, _ := LoadConfigAtPath(path)
	if cfg2.Settings.ProviderPrefix != "second" {
		t.Fatalf("second = %q, want hot reload", cfg2.Settings.ProviderPrefix)
	}
}
