package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func TestTUI_RendersProfiles(t *testing.T) {
	cfg := config.DefaultConfig()
	view := RenderView(cfg)
	if !strings.Contains(view, "Profiles") {
		t.Fatalf("view missing Profiles: %q", view)
	}
	if !strings.Contains(view, "test-provider") {
		t.Fatalf("view missing profile name: %q", view)
	}
	if !strings.Contains(view, "pi-switch TUI") {
		t.Fatalf("view missing header: %q", view)
	}
}

func TestTUI_RendersGatewayAndStats(t *testing.T) {
	cfg := config.DefaultConfig()
	m := New(cfg)
	// View default is profiles tab; switch to gateway
	// We can directly check model fields
	if m.gatewayPreview == "" {
		t.Fatalf("gatewayPreview empty")
	}
	if !strings.Contains(m.gatewayPreview, "Gateway") {
		t.Fatalf("gatewayPreview missing Gateway: %q", m.gatewayPreview)
	}
	// statsBrief should contain cost formatting marker $ or -
	if m.statsBrief == "" {
		t.Fatalf("statsBrief empty")
	}
	// check that View for gateway tab contains gatewayPreview
	m.tab = 1
	v := m.View()
	if !strings.Contains(v, "Gateway") {
		t.Fatalf("gateway tab view missing: %q", v)
	}
	m.tab = 2
	v2 := m.View()
	if !strings.Contains(v2, "Stats") || !strings.Contains(v2, "Cost") {
		t.Fatalf("stats tab view missing: %q", v2)
	}
}

func TestTUI_EmptyConfig(t *testing.T) {
	cfg := config.PiSwitchConfig{
		Version:  2,
		Profiles: map[string]config.ProviderProfile{},
		Settings: config.DefaultConfig().Settings,
	}
	view := RenderView(cfg)
	if !strings.Contains(view, "(no profiles)") {
		t.Fatalf("empty view missing placeholder: %q", view)
	}
}

func TestTUI_SwitchViaFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	// create two profiles
	cfg := config.DefaultConfig()
	prof2 := cfg.Profiles["test-provider"]
	prof2.API = "openai-responses"
	cfg.Profiles["second"] = prof2
	// save
	if err := saveConfig(cfg, cfgPath); err != nil {
		t.Fatalf("save: %v", err)
	}
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	// reload and render
	loaded, _, _ := config.LoadConfigAtPath(cfgPath)
	m := New(loaded)
	view := m.View()
	if !strings.Contains(view, "second") {
		t.Fatalf("view missing second: %q", view)
	}
	_ = os.Remove(cfgPath)
}
