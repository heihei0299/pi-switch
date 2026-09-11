package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Settings.WriteMode != "gateway" {
		t.Fatalf("writeMode = %q, want gateway", cfg.Settings.WriteMode)
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
	if len(cfg.Profiles["test-provider"].Upstreams) != 1 {
		t.Fatalf("default profile must contain one channel")
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
	if err := os.WriteFile(path, []byte(legacy2), 0644); err != nil {
		t.Fatal(err)
	}
	cfg2, _, err := LoadConfigAtPath(path)
	if err != nil {
		t.Fatalf("load legacy2: %v", err)
	}
	if cfg2.Settings.ConversationSource != "off" {
		t.Fatalf("conversationSource = %q, want off", cfg2.Settings.ConversationSource)
	}
}

// ARCH-01: reading a config must fail explicitly. Only a missing file yields
// DefaultConfig; malformed JSON, unreadable files and wrong field types must
// surface as errors so no caller mistakes them for a default/empty config.
func TestLoadConfigAtPath_StrictErrors(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	if err := os.Mkdir(filepath.Join(dir, "as-dir"), 0755); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"malformed json":             write("malformed.json", `{"profiles":`),
		"version wrong type":         write("version.json", `{"version":"two"}`),
		"current wrong type":         write("current.json", `{"current":42}`),
		"profiles wrong type":        write("profiles.json", `{"profiles":[]}`),
		"profile field wrong type":   write("profile.json", `{"profiles":{"p":{"apiKey":42}}}`),
		"settings wrong type":        write("settings.json", `{"settings":"nope"}`),
		"proxy field wrong type":     write("proxy.json", `{"settings":{"proxy":{"port":"x"}}}`),
		"writeMode wrong type":       write("writemode.json", `{"settings":{"writeMode":7}}`),
		"invalid conversationSource": write("conv.json", `{"settings":{"conversationSource":"bogus"}}`),
		"exposedModels null":         write("exposed-null.json", `{"profiles":{"p":{"upstreams":[{"name":"m","exposedModels":null}]}}}`),
		"unreadable path":            filepath.Join(dir, "as-dir"),
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			cfg, _, err := LoadConfigAtPath(path)
			if err == nil {
				t.Fatalf("LoadConfigAtPath(%s) returned nil error, want explicit failure", path)
			}
			if cfg.Current != nil || cfg.Profiles != nil {
				t.Fatalf("failed load must not return a config, got %+v", cfg)
			}
		})
	}
}

// Partial settings must still receive the defaults owned by DefaultConfig and
// Settings.UnmarshalJSON; the loader no longer re-applies them itself.
func TestLoadConfigAtPath_PartialSettingsKeepDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"version":2,"profiles":{},"settings":{"writeMode":"proxy"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := LoadConfigAtPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Settings.WriteMode != "proxy" {
		t.Fatalf("writeMode = %q, want proxy", cfg.Settings.WriteMode)
	}
	if cfg.Settings.ConversationSource != "sessionScan" {
		t.Fatalf("conversationSource = %q, want sessionScan", cfg.Settings.ConversationSource)
	}
	if cfg.Settings.Proxy.Host != "127.0.0.1" || cfg.Settings.Proxy.Port != 43112 {
		t.Fatalf("proxy defaults lost: %+v", cfg.Settings.Proxy)
	}
	if cfg.Settings.Web.Host != "127.0.0.1" || cfg.Settings.Web.Port != 43110 {
		t.Fatalf("web defaults lost: %+v", cfg.Settings.Web)
	}
	cb := cfg.Settings.Proxy.CircuitBreaker
	if !cb.Enabled || cb.FailureThreshold != 3 || cb.CooldownSeconds != 60 {
		t.Fatalf("circuit breaker defaults lost: %+v", cb)
	}
}

// system-contract 2.2: a config file without `profiles` (and one with an explicit
// null) has no profiles. The DefaultConfig placeholder belongs to the missing-file
// default only, never to an explicit config that omits the key.
func TestLoadConfigAtPath_MissingOrNullProfilesIsEmpty(t *testing.T) {
	for _, body := range []string{
		`{"version":2}`,
		`{"version":2,"profiles":null}`,
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		cfg, _, err := LoadConfigAtPath(path)
		if err != nil {
			t.Fatalf("load %s: %v", body, err)
		}
		if cfg.Profiles == nil || len(cfg.Profiles) != 0 {
			t.Fatalf("profiles for %s = %#v, want empty map", body, cfg.Profiles)
		}
	}
}

// system-contract 2.2: `exposedModels: null` is a boundary error and must report
// the full field path so the operator can find the channel.
func TestLoadConfigAtPath_NullExposedModelsNamesField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"profiles":{"p":{"upstreams":[{"name":"m","exposedModels":null}]}}}`), 0644); err != nil {
		t.Fatal(err)
	}
	_, _, err := LoadConfigAtPath(path)
	if err == nil {
		t.Fatal("exposedModels:null loaded without error")
	}
	for _, want := range []string{"profiles.p", "upstreams[0]", "exposedModels"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q is missing path segment %q", err, want)
		}
	}
}
