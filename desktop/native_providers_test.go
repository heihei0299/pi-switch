package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/config"
)

func newProviderPageFixture(t *testing.T) (*desktopShell, *nativeProvidersPage, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Profiles = map[string]config.ProviderProfile{"supplier": {API: "openai-completions", APIKey: "fixture-secret"}}
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", path)
	app := &desktopShell{}
	pages := app.standardPages()
	pages.page = pageProviders
	return app, &pages.providers, path
}

func (p *nativeProvidersPage) fixtureDraft(name string) {
	p.cancelDraft()
	p.original, p.name, p.revision, p.editing = name, name, "revision-1", true
	p.copyName, p.fetchChannel, p.keyChannel = name+"-copy", "primary", "primary"
	p.draft = `{"api": "openai-completions", "baseUrl": "https://example.test/v1"}`
}

func TestNativeProviderListHidesEditingTools(t *testing.T) {
	app, p, _ := newProviderPageFixture(t)
	p.fixtureDraft("")
	p.editing = false
	tester := ui.NewTester(app.standardView, 1000, 1400)
	for _, want := range []string{"Find provider", "Provider list", "Add provider…", "Edit selected provider", "Switch selected profile", "Refresh providers"} {
		if !tester.HasText(want) {
			t.Fatalf("list state is missing %q: %q", want, tester.Texts())
		}
	}
	for _, hidden := range []string{"Provider / channels / models JSON", "Save provider…", "Cancel provider draft", "Add from preset", "Test draft connection", "Delete provider…"} {
		if tester.HasText(hidden) {
			t.Fatalf("list state spreads %q over the list: %q", hidden, tester.Texts())
		}
	}
}

func TestNativeProviderAddFlowOffersBlankAndPreset(t *testing.T) {
	app, p, path := newProviderPageFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tester := ui.NewTester(app.standardView, 1000, 1400)
	if err := tester.Click("Add provider…"); err != nil {
		t.Fatal(err)
	}
	if !p.adding || p.editing {
		t.Fatalf("add flow did not open: adding=%v editing=%v", p.adding, p.editing)
	}
	for _, want := range []string{"Add blank provider", "Provider preset", "Add from preset", "Cancel add"} {
		if !tester.HasText(want) {
			t.Fatalf("add flow is missing %q: %q", want, tester.Texts())
		}
	}
	if tester.HasText("Find provider") {
		t.Fatalf("add flow still spreads the list: %q", tester.Texts())
	}
	if err := tester.Click("Cancel add"); err != nil {
		t.Fatal(err)
	}
	if p.adding || p.editing {
		t.Fatal("cancel add did not return to the list")
	}
	if err := tester.Click("Add provider…"); err != nil {
		t.Fatal(err)
	}
	p.preset = "openai"
	tester.Frame()
	if err := tester.Click("Add from preset"); err != nil {
		t.Fatal(err)
	}
	if !p.editing || p.adding || !strings.Contains(p.draft, `"preset": "openai"`) {
		t.Fatalf("preset draft missing: adding=%v editing=%v", p.adding, p.editing)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("adding a preset draft wrote config")
	}
}

func TestNativeProviderEditHighlightsSaveAndCancel(t *testing.T) {
	app, p, _ := newProviderPageFixture(t)
	p.fixtureDraft("supplier")
	tester := ui.NewTester(app.standardView, 1000, 1400)
	for _, want := range []string{"Provider / channels / models JSON", "Save provider…", "Cancel provider draft", "Danger zone: delete provider", "More provider actions"} {
		if !tester.HasText(want) {
			t.Fatalf("edit state is missing %q: %q", want, tester.Texts())
		}
	}
	for _, hidden := range []string{"Find provider", "Add provider…", "Test draft connection", "Delete provider…"} {
		if tester.HasText(hidden) {
			t.Fatalf("edit state spreads %q: %q", hidden, tester.Texts())
		}
	}
	p.message = "Connection OK · 12 ms. No config was written."
	tester.Frame()
	if !tester.HasText("Connection OK · 12 ms. No config was written.") {
		t.Fatalf("operation result is hidden in edit state: %q", tester.Texts())
	}
}

func TestNativeProviderAuxiliaryActionsAreGroupedAndLazy(t *testing.T) {
	app, p, _ := newProviderPageFixture(t)
	p.fixtureDraft("supplier")
	requested := 0
	app.taskRequested = func(string) { requested++ }
	tester := ui.NewTester(app.standardView, 1000, 2200)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	if err := tester.Click("More provider actions"); err != nil {
		t.Fatal(err)
	}
	if p.busy || requested != 0 {
		t.Fatal("expanding provider tools started a request")
	}
	for _, want := range []string{"Against the draft", "Validate provider", "Test draft connection", "Fetch models into draft", "Against the saved provider", "Refresh saved provider usage", "Duplicate saved provider…"} {
		if !tester.HasText(want) {
			t.Fatalf("auxiliary actions are missing %q: %q", want, tester.Texts())
		}
	}

	p.cancelDraft()
	p.newDraft()
	tester.Frame()
	if err := tester.Click("More provider actions"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Against the draft") || !tester.HasText("Save the provider to use saved-provider actions.") {
		t.Fatalf("new draft did not mark saved-only actions: %q", tester.Texts())
	}
	for _, hidden := range []string{"Refresh saved provider usage", "Duplicate saved provider…"} {
		if tester.HasText(hidden) {
			t.Fatalf("new draft offers saved-provider action %q", hidden)
		}
	}
}

func TestNativeProviderDeleteStaysInDangerZoneWithConfirmation(t *testing.T) {
	app, p, path := newProviderPageFixture(t)
	p.fixtureDraft("supplier")
	tester := ui.NewTester(app.standardView, 1000, 1600)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	if tester.HasText("Delete provider…") {
		t.Fatalf("delete is not behind the danger zone: %q", tester.Texts())
	}
	if err := tester.Click("Danger zone: delete provider"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Delete provider…") {
		t.Fatalf("danger zone is missing delete: %q", tester.Texts())
	}
	if err := tester.Click("Delete provider…"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Delete provider?") || !p.confirm {
		t.Fatal("delete skipped confirmation")
	}
	if err := tester.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if p.confirm || p.busy || p.draft == "" || p.original != "supplier" {
		t.Fatal("cancel delete lost the draft")
	}
	if _, err := os.ReadFile(path); err != nil {
		t.Fatal(err)
	}
}

func TestNativeProviderCancelEditReturnsToListExplicitly(t *testing.T) {
	app, p, path := newProviderPageFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p.fixtureDraft("supplier")
	tester := ui.NewTester(app.standardView, 1000, 1600)
	if err := tester.Click("Cancel provider draft"); err != nil {
		t.Fatal(err)
	}
	if p.editing || p.adding || p.draft != "" || p.original != "" {
		t.Fatal("cancel did not discard the draft")
	}
	if !tester.HasText("Draft discarded. No config or backup was written.") || !tester.HasText("Find provider") {
		t.Fatalf("cancel did not return to the list: %q", tester.Texts())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("canceling the draft wrote config")
	}
}

func TestNativeProviderInvalidDraftBlocksSaveAndKeepsDraft(t *testing.T) {
	app, p, _ := newProviderPageFixture(t)
	p.fixtureDraft("supplier")
	p.draft = "{ invalid"
	tester := ui.NewTester(app.standardView, 1000, 1600)
	if !tester.HasText("Invalid JSON; draft retained and Save disabled.") {
		t.Fatalf("invalid JSON is not reported: %q", tester.Texts())
	}
	if err := tester.Click("Save provider…"); err != nil {
		t.Fatal(err)
	}
	if p.confirm || p.draft != "{ invalid" {
		t.Fatal("invalid draft reached confirmation or was replaced")
	}
}
