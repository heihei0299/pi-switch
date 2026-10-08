package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/config"
)

func TestLoadNativeOverviewUsesSharedConfigAndOmitsCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	current := "中文供应商"
	channel := "primary"
	cfg.Current = &current
	cfg.Profiles = map[string]config.ProviderProfile{
		current: {
			API: "openai-completions",
			Upstreams: []config.Upstream{{
				Name:          &channel,
				BaseURL:       "https://url-user-secret:url-pass-secret@supplier.example:8443/v1?token=url-query-secret",
				APIKey:        "test-secret-must-not-render",
				Models:        []config.ModelEntry{{ID: "model-a"}, {ID: "model-b"}},
				ExposedModels: []string{"model-a"},
			}},
		},
	}
	cfg.Settings.Proxy.Host = "2001:db8::1"
	cfg.Settings.Proxy.Port = 43112
	cfg.Settings.Web.Host = "127.0.0.1"
	cfg.Settings.Web.Port = 43110
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", path)

	overview, err := loadNativeOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.proxyAddress != "[2001:db8::1]:43112" || overview.webAddress != "127.0.0.1:43110" {
		t.Fatalf("settings summary = %q / %q", overview.proxyAddress, overview.webAddress)
	}
	if len(overview.profiles) != 1 || overview.profiles[0].name != current || !overview.profiles[0].current || overview.profiles[0].api != "openai-completions" || overview.profiles[0].endpoint != "https://supplier.example:8443" || overview.profiles[0].models != 2 || overview.profiles[0].exposed != 1 {
		t.Fatalf("profiles summary = %#v", overview.profiles)
	}

	app := &desktopShell{overview: overview, proxyMessage: "Proxy daemon is not running"}
	tester := ui.NewTester(app.view, 760, 600)
	text := strings.Join(tester.Texts(), " ")
	if !tester.HasText(current+" (current)") || !tester.HasText("API: openai-completions") || !tester.HasText("Upstream: https://supplier.example:8443") || !tester.HasText("Models: 2 (1 exposed) · 1 channel(s)") {
		t.Fatalf("active profile summary missing from: %q", tester.Texts())
	}
	for _, secret := range []string{"test-secret-must-not-render", "url-user-secret", "url-pass-secret", "url-query-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("native overview exposed %q in %q", secret, text)
		}
	}
	if !tester.HasText("Proxy: [2001:db8::1]:43112") || !tester.HasText("Web UI: 127.0.0.1:43110") {
		t.Fatalf("settings summary missing from %q", tester.Texts())
	}
}

func TestSwitchNativeProfilePersistsOnlyCurrentAndRejectsStaleSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Profiles = map[string]config.ProviderProfile{
		"first":  {API: "openai-completions", APIKey: "secret-one"},
		"second": {API: "openai-completions", APIKey: "secret-two"},
	}
	current := "first"
	cfg.Current = &current
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", path)

	if err := setNativeCurrentProfile("second"); err != nil {
		t.Fatal(err)
	}
	persisted, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Current == nil || *persisted.Current != "second" {
		t.Fatalf("current profile = %v, want second", persisted.Current)
	}
	if persisted.Profiles["first"].APIKey != "secret-one" || persisted.Profiles["second"].APIKey != "secret-two" {
		t.Fatal("switch changed profile credentials")
	}
	if err := setNativeCurrentProfile("removed"); err == nil {
		t.Fatal("stale profile selection unexpectedly succeeded")
	}
	persisted, _, err = config.LoadConfigAtPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Current == nil || *persisted.Current != "second" {
		t.Fatalf("failed stale switch changed current profile: %v", persisted.Current)
	}
}

func TestNativeOverviewRefreshErrorKeepsLastGoodSnapshot(t *testing.T) {
	previous := nativeOverview{
		profiles:     []overviewProfile{{name: "supplier", current: true}, {name: "backup"}},
		proxyAddress: "127.0.0.1:43112",
		webAddress:   "127.0.0.1:43110",
	}
	app := &desktopShell{
		overview:        previous,
		overviewBusy:    true,
		selectedProfile: 1,
		overviewError:   "old error",
	}
	app.updateNativeOverview(nativeOverview{}, fmt.Errorf("reload failed"))
	if !reflect.DeepEqual(app.overview, previous) || app.overviewError != "reload failed" || app.overviewBusy || app.selectedProfile != 1 {
		t.Fatalf("failed refresh discarded prior state: overview=%#v error=%q busy=%v selected=%d", app.overview, app.overviewError, app.overviewBusy, app.selectedProfile)
	}
	tester := ui.NewTester(app.view, 760, 600)
	if !tester.HasText("supplier (current)") || !tester.HasText("reload failed") {
		t.Fatalf("refresh error did not preserve the visible overview: %q", tester.Texts())
	}

	updated := nativeOverview{profiles: []overviewProfile{{name: "new supplier"}}}
	app.updateNativeOverview(updated, nil)
	if !reflect.DeepEqual(app.overview, updated) || app.overviewError != "" || app.overviewBusy || app.selectedProfile != -1 {
		t.Fatalf("successful refresh did not replace/reset state: overview=%#v error=%q busy=%v selected=%d", app.overview, app.overviewError, app.overviewBusy, app.selectedProfile)
	}
}

func TestNativeOverviewFiltersProfilesWithIMEComposition(t *testing.T) {
	app := &desktopShell{overview: nativeOverview{profiles: []overviewProfile{
		{name: "English supplier"},
		{name: "中文供应商", current: true},
	}}}
	tester := ui.NewTester(app.view, 760, 600)
	if err := tester.Click("Filter profiles"); err != nil {
		t.Fatal(err)
	}
	tester.Compose("zhong", 5)
	if app.profileQuery != "" {
		t.Fatalf("uncommitted IME composition changed the query to %q", app.profileQuery)
	}
	tester.Type("中文")
	if app.profileQuery != "中文" || !tester.HasText("中文供应商 (current)") || tester.HasText("English supplier") {
		t.Fatalf("IME-filtered profiles = %q, text %q", app.profileQuery, tester.Texts())
	}
	tester.SetDark(true)
	tester.Frame()
	if !tester.HasText("中文供应商 (current)") {
		t.Fatalf("profile missing in dark theme: %q", tester.Texts())
	}
}

func TestNativeOverviewVirtualizesAndNavigatesLongProfileList(t *testing.T) {
	profiles := make([]overviewProfile, 1000)
	for i := range profiles {
		profiles[i] = overviewProfile{name: fmt.Sprintf("Profile %04d", i)}
	}
	profiles[0].name = strings.Repeat("Long供应商", 24)
	app := &desktopShell{overview: nativeOverview{profiles: profiles}}
	tester := ui.NewTester(app.view, 760, 600)
	if !tester.HasText(profiles[0].name) || tester.HasText(profiles[len(profiles)-1].name) {
		t.Fatalf("list did not render only the visible rows: %q", tester.Texts())
	}
	tester.SetScale(2)
	tester.Frame()
	if !tester.HasText(profiles[0].name) {
		t.Fatalf("first row disappeared at scale 2: %q", tester.Texts())
	}
	if err := tester.Click(profiles[0].name); err != nil {
		t.Fatal(err)
	}
	tester.Key(0, ui.KeyEnd)
	if app.selectedProfile != len(profiles)-1 || !tester.HasText(profiles[len(profiles)-1].name) {
		t.Fatalf("End did not focus the last row: selected=%d text=%q", app.selectedProfile, tester.Texts())
	}
	tester.Key(0, ui.KeyHome)
	if app.selectedProfile != 0 || !tester.HasText(profiles[0].name) {
		t.Fatalf("Home did not focus the first row: selected=%d text=%q", app.selectedProfile, tester.Texts())
	}
	tester.Key(0, ui.KeyDown)
	if app.selectedProfile != 1 || !tester.HasText(profiles[1].name) {
		t.Fatalf("Down did not focus the next row: selected=%d text=%q", app.selectedProfile, tester.Texts())
	}
	tester.Key(0, ui.KeyUp)
	if app.selectedProfile != 0 || !tester.HasText(profiles[0].name) {
		t.Fatalf("Up did not focus the previous row: selected=%d text=%q", app.selectedProfile, tester.Texts())
	}
}

func TestNativeOverviewEmptyAndInvalidConfigStates(t *testing.T) {
	app := &desktopShell{}
	tester := ui.NewTester(app.view, 760, 600)
	if !tester.HasText("No profiles configured.") {
		t.Fatalf("empty state missing from %q", tester.Texts())
	}

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", path)
	if _, err := loadNativeOverview(); err == nil {
		t.Fatal("invalid config was accepted as an empty overview")
	}
	app.overviewError = "invalid config"
	tester.Frame()
	if !tester.HasText("invalid config") {
		t.Fatalf("config error missing from %q", tester.Texts())
	}
}
