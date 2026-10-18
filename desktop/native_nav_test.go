package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/adapters/outbound/filegateway"
	"github.com/heihei0299/pi-switch/internal/config"
)

func newNavigationShell(t *testing.T) (*desktopShell, *nativeStandardPages, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	current := "supplier"
	cfg.Current = &current
	cfg.Profiles = map[string]config.ProviderProfile{current: {API: "openai-completions"}}
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", path)
	// The Gateway writes its own models file, so point that at the temporary
	// directory too; otherwise a publish would land in the real user data.
	t.Setenv("PI_SWITCH_MODELS", filepath.Join(filepath.Dir(path), "models.json"))
	app := &desktopShell{complexPreview: newNativeComplexPreview()}
	return app, app.standardPages(), path
}

func TestNativeNavigationIsFourPagesAndMaintenance(t *testing.T) {
	app, pages, _ := newNavigationShell(t)
	tester := ui.NewTester(app.standardView, 1000, 1500)
	row := math.Inf(1)
	for _, entry := range []string{"Overview", "Providers", "Gateway", "Usage & Sessions", "Maintenance"} {
		rect, ok := tester.Find(entry)
		if !ok {
			t.Fatalf("navigation entry %q is missing: %q", entry, tester.Texts())
		}
		if math.IsInf(row, 1) {
			row = float64(rect.Y)
		} else if math.Abs(float64(rect.Y)-row) > 2 {
			t.Fatalf("navigation entry %q is not on the main row: %+v", entry, rect)
		}
	}
	occurrences := 0
	for _, text := range tester.Texts() {
		if text == "Providers" {
			occurrences++
		}
	}
	if occurrences != 1 {
		t.Fatalf("Providers appears %d times in the navigation: %q", occurrences, tester.Texts())
	}
	for _, gone := range []string{"JSON Editor", "Stats & Sessions", "Backups", "Doctor", "Packages", "Settings"} {
		if tester.HasText(gone) {
			t.Fatalf("%q is still a top level entry on the overview: %q", gone, tester.Texts())
		}
	}

	if err := tester.Click("Proxy"); err != nil {
		t.Fatal(err)
	}
	if pages.page != pageOverview || !tester.HasText("Check Proxy Status") {
		t.Fatalf("Proxy is not a section of the overview: page=%s texts=%q", pages.page, tester.Texts())
	}

	// The overview labels its profile list "Profiles", so the removed entry is
	// checked by clicking it and confirming that no navigation happens.
	if err := tester.Click("Profiles"); err != nil {
		t.Fatal(err)
	}
	if pages.page != pageOverview || !tester.HasText("Switch provider") {
		t.Fatalf("Profiles is still a top level entry: page=%s texts=%q", pages.page, tester.Texts())
	}
}

func TestNativeMaintenanceAreaGroupsEntriesByPurpose(t *testing.T) {
	app, pages, _ := newNavigationShell(t)
	pages.backups.loaded = true
	pages.doctor.loaded = true
	tester := ui.NewTester(app.standardView, 1000, 1500)
	if err := tester.Click("Maintenance"); err != nil {
		t.Fatal(err)
	}
	if pages.page != pageMaintenance {
		t.Fatalf("page = %s, want %s", pages.page, pageMaintenance)
	}
	for _, want := range []string{"Configuration", "Recovery", "Extensions", "Diagnostics"} {
		if !tester.HasText(want) {
			t.Fatalf("maintenance group %q is missing: %q", want, tester.Texts())
		}
	}
	for _, want := range []string{"Settings", "Advanced JSON", "Backups", "Packages", "Doctor"} {
		if !tester.HasText(want) {
			t.Fatalf("maintenance entry %q is missing: %q", want, tester.Texts())
		}
	}

	for _, tc := range []struct {
		entry string
		page  string
		want  string
	}{
		{"Settings", pageSettings, "Settings have not loaded yet."},
		{"Advanced JSON", pageAdvancedJSON, "Gateway JSON draft validation"},
		{"Backups", pageBackups, "No backups yet."},
		{"Packages", pagePackages, "Packages · local package registry"},
		{"Doctor", pageDoctor, "No validation issues found."},
	} {
		if err := tester.Click("Maintenance"); err != nil {
			t.Fatal(err)
		}
		if err := tester.Click(tc.entry); err != nil {
			t.Fatal(err)
		}
		if pages.page != tc.page || !tester.HasText(tc.want) {
			t.Fatalf("%s did not open: page=%s texts=%q", tc.entry, pages.page, tester.Texts())
		}
	}
}

func TestNativeNavigationKeepsDraftsAndActsOnNothing(t *testing.T) {
	app, pages, path := newNavigationShell(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pages.providers.fixtureDraft("supplier")
	settingsDraft := `{"web": {"port": 43110}}`
	pages.settings.loaded, pages.settings.draft = true, settingsDraft
	tester := ui.NewTester(app.standardView, 1000, 1500)
	for _, page := range []string{"Gateway", "Usage & Sessions", "Overview"} {
		if err := tester.Click(page); err != nil {
			t.Fatalf("navigate %s: %v", page, err)
		}
	}
	// Settings is a second level entry, reachable through the maintenance area.
	if err := tester.Click("Maintenance"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("Providers"); err != nil {
		t.Fatal(err)
	}

	if !pages.providers.editing || pages.providers.original != "supplier" || pages.providers.draft == "" {
		t.Fatal("navigation discarded the provider draft")
	}
	if !pages.settings.loaded || pages.settings.draft != settingsDraft {
		t.Fatal("navigation discarded the settings draft")
	}
	if app.proxy.status.checked || app.proxy.busy || app.proxy.stopConfirm {
		t.Fatal("navigation touched the proxy")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("navigation wrote the config")
	}
	modelsPath := filegateway.ModelsPath()
	if modelsPath != filepath.Join(filepath.Dir(path), "models.json") {
		t.Fatalf("the Gateway models path is not isolated: %s", modelsPath)
	}
	if _, err := os.Stat(modelsPath); err == nil {
		t.Fatal("navigation published the gateway")
	}
}

func TestNativeNavigationIsKeyboardReachable(t *testing.T) {
	app, pages, _ := newNavigationShell(t)
	tester := ui.NewTester(app.standardView, 1000, 1500)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	tester.Key(0, ui.KeyTab)
	if !tester.Focused("Overview") {
		t.Fatalf("the first tab did not reach the main navigation")
	}
	for i := 0; i < 4 && !tester.Focused("Gateway"); i++ {
		tester.Key(0, ui.KeyTab)
	}
	if !tester.Focused("Gateway") {
		t.Fatalf("Gateway is not keyboard reachable")
	}
	tester.Key(0, ui.KeySpace)
	if pages.page != pageGateway || !tester.HasText("Current vs proposed Gateway") {
		t.Fatalf("Space did not open Gateway: page=%s texts=%q", pages.page, tester.Texts())
	}
	for i := 0; i < 4 && !tester.Focused("Maintenance"); i++ {
		tester.Key(0, ui.KeyTab)
	}
	if !tester.Focused("Maintenance") {
		t.Fatalf("Maintenance is not keyboard reachable")
	}
	tester.Key(0, ui.KeyEnter)
	if pages.page != pageMaintenance {
		t.Fatalf("Enter did not open the maintenance area: %s", pages.page)
	}
	for i := 0; i < 8 && !tester.Focused("Backups"); i++ {
		tester.Key(0, ui.KeyTab)
	}
	if !tester.Focused("Backups") {
		t.Fatalf("second level entries are not keyboard reachable")
	}
	tester.Key(0, ui.KeySpace)
	if pages.page != pageBackups {
		t.Fatalf("Space did not open Backups: %s", pages.page)
	}
}
