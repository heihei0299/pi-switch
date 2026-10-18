package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/adapters/outbound/filegateway"
	"github.com/heihei0299/pi-switch/internal/config"
	domaingateway "github.com/heihei0299/pi-switch/internal/domain/gateway"
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

	app := &desktopShell{overview: nativeOverviewController{snapshot: overview}, proxy: nativeProxyController{message: "Proxy daemon is not running"}}
	// The overview scrolls; connection details sit below the fold of a short window.
	tester := ui.NewTester(app.view, 760, 1000)
	text := strings.Join(tester.Texts(), " ")
	if !tester.HasText(current+" (current)") || !tester.HasText("API: openai-completions") || !tester.HasText("Upstream: https://supplier.example:8443") || !tester.HasText("Models: 2 (1 exposed) · 1 channel(s)") {
		t.Fatalf("active profile summary missing from: %q", tester.Texts())
	}
	for _, secret := range []string{"test-secret-must-not-render", "url-user-secret", "url-pass-secret", "url-query-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("native overview exposed %q in %q", secret, text)
		}
	}
	if tester.HasText("Proxy: [2001:db8::1]:43112") {
		t.Fatalf("connection details takes over the main area: %q", tester.Texts())
	}
	if err := tester.Click("Connection details"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Proxy: [2001:db8::1]:43112") || !tester.HasText("Web UI: 127.0.0.1:43110") {
		t.Fatalf("settings summary missing from %q", tester.Texts())
	}
}

func TestNativeOverviewKeepsErrorsVisibleWithDetailsCollapsed(t *testing.T) {
	app := &desktopShell{overview: nativeOverviewController{error: "config unreadable"}}
	tester := ui.NewTester(app.view, 760, 900)
	if !tester.HasText("config unreadable") {
		t.Fatalf("overview error is hidden: %q", tester.Texts())
	}
	if tester.HasText("Config source: ") {
		t.Fatalf("connection details are not collapsed: %q", tester.Texts())
	}
}

func TestNativeOverviewSwitchFeedbackMatchesCurrentMarker(t *testing.T) {
	app := &desktopShell{overview: nativeOverviewController{
		snapshot:      nativeOverview{profiles: []overviewProfile{{name: "first"}, {name: "second", current: true}}},
		switchMessage: "Switched to second",
	}}
	tester := ui.NewTester(app.view, 760, 900)
	if !tester.HasText("Switched to second") || !tester.HasText("second (current)") || tester.HasText("first (current)") {
		t.Fatalf("switch feedback does not match the current marker: %q", tester.Texts())
	}

	app.overview.switchMessage = "Switch failed: Provider no longer exists; reload before continuing."
	app.overview.snapshot.profiles = []overviewProfile{{name: "first", current: true}, {name: "second"}}
	tester.Frame()
	if !tester.HasText("Switch failed: Provider no longer exists; reload before continuing.") || !tester.HasText("first (current)") || tester.HasText("second (current)") {
		t.Fatalf("failed switch changed the current marker: %q", tester.Texts())
	}
}

func TestNativeOverviewRenderWritesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	current := "supplier"
	cfg.Current = &current
	cfg.Profiles = map[string]config.ProviderProfile{current: {API: "openai-completions"}}
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", path)
	// Point the Gateway's own file at the temporary directory too, so this
	// check cannot pass because a publish went to the real user path.
	t.Setenv("PI_SWITCH_MODELS", filepath.Join(dir, "models.json"))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	app := &desktopShell{}
	tester := ui.NewTester(app.view, 760, 900)
	if err := tester.Click("Connection details"); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("rendering the overview wrote the config")
	}
	modelsPath := filegateway.ModelsPath()
	if modelsPath != filepath.Join(dir, "models.json") {
		t.Fatalf("the Gateway models path is not isolated: %s", modelsPath)
	}
	if _, err := os.Stat(modelsPath); err == nil {
		t.Fatal("rendering the overview published the gateway")
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

	if err := setNativeCurrentProfile(context.Background(), "second"); err != nil {
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
	if err := setNativeCurrentProfile(context.Background(), "removed"); err == nil {
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
	app := &desktopShell{overview: nativeOverviewController{
		snapshot: previous, busy: true, selected: 1, error: "old error",
	}}
	app.overview.update(nativeOverview{}, fmt.Errorf("reload failed"))
	if !reflect.DeepEqual(app.overview.snapshot, previous) || app.overview.error != "reload failed" || app.overview.busy || app.overview.selected != 1 {
		t.Fatalf("failed refresh discarded prior state: overview=%#v error=%q busy=%v selected=%d", app.overview.snapshot, app.overview.error, app.overview.busy, app.overview.selected)
	}
	tester := ui.NewTester(app.view, 760, 600)
	if !tester.HasText("supplier (current)") || !tester.HasText("reload failed") {
		t.Fatalf("refresh error did not preserve the visible overview: %q", tester.Texts())
	}

	updated := nativeOverview{profiles: []overviewProfile{{name: "new supplier"}}}
	app.overview.update(updated, nil)
	if !reflect.DeepEqual(app.overview.snapshot, updated) || app.overview.error != "" || app.overview.busy || app.overview.selected != -1 {
		t.Fatalf("successful refresh did not replace/reset state: overview=%#v error=%q busy=%v selected=%d", app.overview.snapshot, app.overview.error, app.overview.busy, app.overview.selected)
	}
}

func TestNativeOverviewFiltersProfilesWithIMEComposition(t *testing.T) {
	app := &desktopShell{overview: nativeOverviewController{snapshot: nativeOverview{profiles: []overviewProfile{
		{name: "English supplier"},
		{name: "中文供应商", current: true},
	}}}}
	tester := ui.NewTester(app.view, 760, 600)
	if err := tester.Click("Filter profiles"); err != nil {
		t.Fatal(err)
	}
	tester.Compose("zhong", 5)
	if app.overview.profileQuery != "" {
		t.Fatalf("uncommitted IME composition changed the query to %q", app.overview.profileQuery)
	}
	tester.Type("中文")
	if app.overview.profileQuery != "中文" || !tester.HasText("中文供应商 (current)") || tester.HasText("English supplier") {
		t.Fatalf("IME-filtered profiles = %q, text %q", app.overview.profileQuery, tester.Texts())
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
	app := &desktopShell{overview: nativeOverviewController{snapshot: nativeOverview{profiles: profiles}}}
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
	if app.overview.selected != len(profiles)-1 || !tester.HasText(profiles[len(profiles)-1].name) {
		t.Fatalf("End did not focus the last row: selected=%d text=%q", app.overview.selected, tester.Texts())
	}
	tester.Key(0, ui.KeyHome)
	if app.overview.selected != 0 || !tester.HasText(profiles[0].name) {
		t.Fatalf("Home did not focus the first row: selected=%d text=%q", app.overview.selected, tester.Texts())
	}
	tester.Key(0, ui.KeyDown)
	if app.overview.selected != 1 || !tester.HasText(profiles[1].name) {
		t.Fatalf("Down did not focus the next row: selected=%d text=%q", app.overview.selected, tester.Texts())
	}
	tester.Key(0, ui.KeyUp)
	if app.overview.selected != 0 || !tester.HasText(profiles[0].name) {
		t.Fatalf("Up did not focus the previous row: selected=%d text=%q", app.overview.selected, tester.Texts())
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
	app.overview.error = "invalid config"
	tester.Frame()
	if !tester.HasText("invalid config") {
		t.Fatalf("config error missing from %q", tester.Texts())
	}
}

func TestNativeGatewayStatusTextDistinguishesUnknownFromClean(t *testing.T) {
	for _, tc := range []struct {
		name    string
		state   nativeGatewayState
		want    string
		warning bool
	}{
		{"not checked", nativeGatewayState{}, "publish status unknown (not checked yet)", false},
		{"loading", nativeGatewayState{gatewayLoading: true}, "checking for unpublished changes…", false},
		{"failed", nativeGatewayState{gatewayError: "models.json unreadable"}, "publish status unknown: models.json unreadable", true},
		{"stale after failure", nativeGatewayState{gatewayLoaded: true, gatewayError: "reload failed"}, "the last check failed, so this may be stale: reload failed", true},
		{"publishing", nativeGatewayState{gatewayLoaded: true, gatewayPublishing: true}, "publishing the selected models…", false},
		{"pending", nativeGatewayState{gatewayLoaded: true, gateway: nativeGatewayPreview{pendingCount: 2}}, "2 model change(s) ready to publish; publishing stays explicit", false},
		{"conflict", nativeGatewayState{gatewayLoaded: true, gateway: nativeGatewayPreview{pendingCount: 1, conflictCount: 1}}, "1 conflict(s) block publishing; resolve them on the Gateway page", true},
		{"published", nativeGatewayState{gatewayLoaded: true}, "up to date; nothing to publish", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := tc.state
			got, warning := nativeGatewayStatusText(&state)
			if got != tc.want || warning != tc.warning {
				t.Fatalf("status = %q (warning %v), want %q (warning %v)", got, warning, tc.want, tc.warning)
			}
		})
	}
	if got, warning := nativeGatewayStatusText(nil); got != "publish status unknown (not checked yet)" || warning {
		t.Fatalf("a missing state was reported as %q (warning %v)", got, warning)
	}
}

func TestNativeOverviewGatewayHintNavigatesWithoutPublishing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := config.DefaultConfig()
	current := "supplier"
	cfg.Current = &current
	cfg.Profiles = map[string]config.ProviderProfile{current: {API: "openai-completions"}}
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", path)
	t.Setenv("PI_SWITCH_MODELS", filepath.Join(dir, "models.json"))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	app := &desktopShell{complexPreview: newNativeComplexPreview()}
	gateway := &app.complexPreview.nativeGatewayState
	gateway.gatewayLoaded = true
	gateway.gateway.pendingCount = 3
	published := false
	gateway.publishGateway = func([]domaingateway.GatewaySelection) { published = true }
	refreshed := 0
	gateway.refreshGateway = func([]domaingateway.GatewaySelection) { refreshed++ }

	tester := ui.NewTester(app.standardView, 900, 1400)
	if !tester.HasText("Gateway: 3 model change(s) ready to publish; publishing stays explicit") {
		t.Fatalf("the overview hides the pending publish: %q", tester.Texts())
	}
	if err := tester.Click("Open Gateway"); err != nil {
		t.Fatal(err)
	}
	if app.standardPages().page != pageGateway {
		t.Fatalf("the hint did not open the Gateway page: page=%s", app.standardPages().page)
	}
	if published {
		t.Fatal("opening the Gateway from the overview published the selection")
	}
	if refreshed == 0 {
		t.Fatal("opening the Gateway did not refresh the shared preview")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("the publish hint wrote the config")
	}
	if _, err := os.Stat(filegateway.ModelsPath()); err == nil {
		t.Fatal("the publish hint published models.json")
	}
}

func TestNativeOverviewGatewayRefreshKeepsPageSelection(t *testing.T) {
	app := &desktopShell{complexPreview: newNativeComplexPreview()}
	gateway := &app.complexPreview.nativeGatewayState
	gateway.gateway = nativeGatewayPreview{candidates: []nativeGatewayCandidate{{
		key:       "supplier\x00main\x00model-a1",
		selection: domaingateway.GatewaySelection{Supplier: "supplier", Channel: "main", Model: "model-a1"},
		selected:  true,
	}}}
	var got []domaingateway.GatewaySelection
	calls := 0
	gateway.refreshGateway = func(selections []domaingateway.GatewaySelection) { calls++; got = selections }

	app.refreshGatewaySummary()
	if calls != 1 || got != nil {
		t.Fatalf("the default selection must ask for the default plan: calls=%d selections=%#v", calls, got)
	}
	gateway.gatewaySelectionExplicit = true
	gateway.gateway.candidates[0].selected = false
	app.refreshGatewaySummary()
	if got == nil || len(got) != 0 {
		t.Fatalf("an explicit empty selection must stay explicit: %#v", got)
	}
	gateway.gateway.candidates[0].selected = true
	app.refreshGatewaySummary()
	if len(got) != 1 || got[0].Model != "model-a1" {
		t.Fatalf("an unpublished selection was not passed through: %#v", got)
	}
}

