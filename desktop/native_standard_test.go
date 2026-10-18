package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/application"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/profile"
	"github.com/heihei0299/pi-switch/internal/usecase/proxycontrol"
)

func TestNativeStandardNavigationAndEmptyStates(t *testing.T) {
	app := &desktopShell{complexPreview: newNativeComplexPreview()}
	p := app.standardPages()
	p.backups.loaded = true
	p.doctor.loaded = true
	tester := ui.NewTester(app.standardView, 560, 1200)
	for _, tc := range []struct{ page, want string }{
		{pageProviders, "No providers match. Add a provider or clear the filter."},
		{pageGateway, "Current vs proposed Gateway"},
		{pageUsage, "Usage summary · local request history"},
		{pageOverview, "Status not checked yet."},
	} {
		if err := tester.Click(tc.page); err != nil {
			t.Fatalf("navigate %s: %v", tc.page, err)
		}
		if p.page != tc.page || !tester.HasText(tc.want) {
			t.Fatalf("page = %s, want %s with %q in %q", p.page, tc.page, tc.want, tester.Texts())
		}
	}
	for _, tc := range []struct{ entry, page, want string }{
		{pageSettings, pageSettings, "Settings have not loaded yet."},
		{pageAdvancedJSON, pageAdvancedJSON, "Gateway JSON draft validation"},
		{pageBackups, pageBackups, "No backups yet."},
		{pagePackages, pagePackages, "Packages · local package registry"},
		{pageDoctor, pageDoctor, "No validation issues found."},
	} {
		if err := tester.Click("Maintenance"); err != nil {
			t.Fatal(err)
		}
		if err := tester.Click(tc.entry); err != nil {
			t.Fatalf("navigate %s: %v", tc.entry, err)
		}
		if p.page != tc.page || !tester.HasText(tc.want) {
			t.Fatalf("page = %s, want %s with %q in %q", p.page, tc.page, tc.want, tester.Texts())
		}
	}
}

func TestNativeProviderDraftMasksSecretsAndCancelWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Profiles = map[string]config.ProviderProfile{"supplier": {
		API: "openai-completions", APIKey: "fake-saved-key", BaseURL: "https://fake-user:fake-password@example.test/v1?secret=fake-query-key",
		Headers: map[string]string{"Authorization": "fake-saved-header"},
	}}
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", path)
	before, _ := os.ReadFile(path)
	draft, err := application.LoadProviderDraft(path, "supplier")
	if err != nil {
		t.Fatal(err)
	}
	app := &desktopShell{}
	pages := app.standardPages()
	pages.page = pageProviders
	p := &pages.providers
	p.original, p.name, p.draft, p.revision, p.editing = "supplier", "supplier", draft.JSON, draft.Revision, true
	p.key = "fake-replacement-key"
	tester := ui.NewTester(app.standardView, 1000, 1800)
	text := strings.Join(tester.Texts(), " ")
	for _, secret := range []string{"fake-saved-key", "fake-replacement-key", "fake-password", "fake-query-key", "fake-saved-header"} {
		if strings.Contains(text, secret) {
			t.Fatalf("widget exposed %s", secret)
		}
	}
	if err := tester.Click("Save provider…"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Save provider?") {
		t.Fatal("save skipped confirmation")
	}
	if err := tester.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if p.draft != draft.JSON || p.key != "fake-replacement-key" {
		t.Fatal("cancel confirmation discarded draft")
	}
	if err := tester.Click("Cancel provider draft"); err != nil {
		t.Fatal(err)
	}
	if p.editing || p.key != "" || p.draft != "" {
		t.Fatal("cancel did not clear credentials from draft")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("cancel wrote config")
	}
	items, err := application.ListConfigBackups(path)
	if err != nil || len(items) != 0 {
		t.Fatal("cancel created backup")
	}
}

func TestNativeInvalidDraftAndSettingsCancelRetainData(t *testing.T) {
	app := &desktopShell{}
	pages := app.standardPages()
	pages.page = pageSettings
	settings := config.DefaultConfig().Settings
	data, _ := json.MarshalIndent(settings, "", "  ")
	pages.settings.loaded, pages.settings.draft = true, string(data)
	tester := ui.NewTester(app.standardView, 1000, 1500)
	if err := tester.Click("Save settings…"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Save settings?") {
		t.Fatal("settings skipped confirmation")
	}
	if err := tester.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if pages.settings.draft != string(data) {
		t.Fatal("settings cancel lost draft")
	}
	pages.settings.draft = "{ invalid"
	tester.Frame()
	if !tester.HasText("Invalid JSON; draft retained. Nothing has been saved.") {
		t.Fatal("invalid settings state missing")
	}
	pages.page = pageProviders
	pages.providers.newDraft()
	pages.providers.draft = "{ invalid"
	tester.Frame()
	if !tester.HasText("Invalid JSON; draft retained and Save disabled.") {
		t.Fatal("invalid provider state missing")
	}
	if err := tester.Click("Save provider…"); err != nil {
		t.Fatal(err)
	}
	if pages.providers.confirm {
		t.Fatal("invalid draft enabled save")
	}
}

func TestNativeStopProxyRequiresConfirmation(t *testing.T) {
	app := &desktopShell{}
	pid := uint32(4242)
	app.proxy.status = nativeProxyStatus{checked: true, result: proxycontrol.Result{Running: true, Pid: &pid}}
	tester := ui.NewTester(app.standardView, 1000, 900)
	if err := tester.Click("Stop Proxy"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Stop managed Proxy?") || !app.proxy.stopConfirm {
		t.Fatal("stop bypassed confirmation")
	}
	if err := tester.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if app.proxy.stopConfirm || app.proxy.busy {
		t.Fatal("cancel attempted to stop Proxy")
	}
}

func TestNativeDiagnosticReadFailureDoesNotRenderRawSecret(t *testing.T) {
	app := &desktopShell{}
	pages := app.standardPages()
	pages.page = pageDoctor
	pages.doctor.loaded = true
	pages.doctor.result = nativeDoctorResult{
		checks: []application.DoctorCheck{{OK: false, Message: "parse error fake-sensitive-value"}},
		issues: []application.ValidationIssue{{Level: "error", Path: "config", Message: "parse error fake-sensitive-value"}},
	}
	tester := ui.NewTester(app.standardView, 1000, 900)
	if strings.Contains(strings.Join(tester.Texts(), " "), "fake-sensitive-value") {
		t.Fatal("doctor leaked parse input")
	}
	if !strings.Contains(nativeManagementError(errors.New("fake-sensitive-value")), "Operation failed") {
		t.Fatal("missing failure state")
	}
	if strings.Contains(nativeManagementError(errors.New("fake-sensitive-value")), "fake-sensitive-value") {
		t.Fatal("status leaked error input")
	}
}

func TestNativeProviderPresetAndDuplicateConfirmationAreReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PI_SWITCH_CONFIG", path)
	app := &desktopShell{}
	pages := app.standardPages()
	pages.page = pageProviders
	p := &pages.providers
	p.preset = "openai"
	tester := ui.NewTester(app.standardView, 1000, 2100)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	if err := tester.Click("Add provider…"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("Add from preset"); err != nil {
		t.Fatal(err)
	}
	if !p.editing || p.fetchChannel != "primary" || !strings.Contains(p.draft, `"preset": "openai"`) || p.key != "" {
		t.Fatal("preset did not produce safe draft")
	}
	var prof config.ProviderProfile
	if err := json.Unmarshal([]byte(p.draft), &prof); err != nil {
		t.Fatal(err)
	}
	if prof.Upstreams[0].ExposedModels == nil || len(prof.Upstreams[0].ExposedModels) != 0 {
		t.Fatal("preset auto-exposed models")
	}
	p.original, p.copyName = "supplier", "copy"
	p.usage = []application.ProviderUsageWindow{{Name: "rolling", Status: "unknown"}}
	tester.Frame()
	if err := tester.Click("More provider actions"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("rolling: unknown · unknown") {
		t.Fatal("unknown usage displayed as zero")
	}
	if err := tester.Click("Duplicate saved provider…"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Duplicate saved provider?") {
		t.Fatal("copy bypassed confirmation")
	}
	if err := tester.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if !p.editing || p.copyName != "copy" || p.confirm {
		t.Fatal("copy cancel changed draft")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("preset/copy cancellation wrote config")
	}
}

func TestNativeProviderNetworkControlsBlockInvalidDraft(t *testing.T) {
	app := &desktopShell{}
	pages := app.standardPages()
	pages.page = pageProviders
	p := &pages.providers
	p.newDraft()
	p.draft = "{ invalid"
	requested := 0
	app.taskRequested = func(string) { requested++ }
	tester := ui.NewTester(app.standardView, 1000, 1800)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	if err := tester.Click("More provider actions"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"Test draft connection", "Fetch models into draft"} {
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
		if p.busy || requested != 0 {
			t.Fatalf("invalid draft started a network action via %s", label)
		}
	}
	zero := float64(0)
	if got := nativeUsageWindowText(application.ProviderUsageWindow{Name: "weekly", Percent: &zero, Status: "ok"}); !strings.Contains(got, "0.00%") {
		t.Fatalf("known zero did not remain zero: %q", got)
	}
}

func TestNativeProxySnapshotDistinguishesUnknownIdentityAndActualAddress(t *testing.T) {
	app := &desktopShell{}
	tester := ui.NewTester(app.view, 1000, 800)
	if !tester.HasText("Proxy identity and health: unknown (not checked).") {
		t.Fatal("unchecked identity was healthy")
	}
	pid, host, port := uint32(12345), "2001:db8::1", uint16(1234)
	app.proxy.status = nativeProxyStatus{checked: true, checkedAt: "2026-10-10T22:00:00-07:00", result: proxycontrol.Result{Running: true, Pid: &pid, Host: &host, Port: &port}}
	tester.Frame()
	if err := tester.Click("Proxy details"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Managed PID: 12345") || !tester.HasText("Actual listener: [2001:db8::1]:1234") {
		t.Fatal("actual listener identity missing")
	}
	app.proxy.status.failed = true
	tester.Frame()
	if tester.HasText("Managed Proxy identity and health verified.") || !tester.HasText("Proxy identity and health: unknown (check failed).") {
		t.Fatal("failed refresh reported verified identity")
	}
	if strings.Contains(nativeManagementError(fmt.Errorf("wrapped: %w", proxycontrol.ErrPortInUse)), "takeover") == false {
		t.Fatal("port owner refusal missing")
	}
}

func TestNativeSettingsShowsNativeBinaryIdentity(t *testing.T) {
	app := &desktopShell{}
	app.standardPages().page = pageSettings
	tester := ui.NewTester(app.standardView, 560, 1200)
	if !tester.HasText(nativeBuildIdentityText()) || !strings.Contains(nativeBuildIdentityText(), "target linux/") {
		t.Fatal("Native build facts missing")
	}
}

func TestNativeProviderCompositionBlocksSaveDuplicateAndDelete(t *testing.T) {
	app := &desktopShell{}
	pages := app.standardPages()
	pages.page = pageProviders
	p := &pages.providers
	p.newDraft()
	p.original, p.name, p.copyName = "supplier", "supplier", "copy"
	tester := ui.NewTester(app.standardView, 1000, 2400)
	tester.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	if err := tester.Click("More provider actions"); err != nil {
		t.Fatal(err)
	}
	if err := tester.Click("Danger zone: delete provider"); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"Provider name", "Saved provider copy name"} {
		for _, button := range []string{"Save provider…", "Duplicate saved provider…", "Delete provider…"} {
			if err := tester.Click(field); err != nil {
				t.Fatal(err)
			}
			tester.Compose("中文供应商", 3)
			if !p.composing {
				t.Fatalf("composition in %s was not tracked", field)
			}
			if err := tester.Click(button); err != nil {
				t.Fatal(err)
			}
			if p.confirm {
				t.Fatalf("IME preedit triggered write confirmation: field=%s button=%s", field, button)
			}
			tester.Compose("", 0)
		}
	}
}

// The pages above assert that no task started. Without a window the task
// helpers must refuse before they touch the task registry, which is what makes
// those assertions meaningful in a headless tester.
func TestNativeTaskHelpersRefuseWithoutAWindow(t *testing.T) {
	app := &desktopShell{}
	ran := false
	if app.runStandardTask("provider", func(context.Context) (interface{}, error) {
		ran = true
		return nil, nil
	}, func(interface{}, error) {}) {
		t.Fatal("runStandardTask reported a start without a window")
	}
	if ran {
		t.Fatal("runStandardTask ran the task without a window")
	}
	proxy := &nativeProxyController{}
	proxy.runTask(app, func() (proxycontrol.Result, error) {
		ran = true
		return proxycontrol.Result{}, nil
	})
	if ran || proxy.busy {
		t.Fatal("the proxy task ran without a window")
	}
}

func TestNativeSettingsPreeditBlurCannotTriggerSave(t *testing.T) {
	app := &desktopShell{}
	pages := app.standardPages()
	pages.page = pageSettings
	data, _ := json.Marshal(config.DefaultConfig().Settings)
	pages.settings.loaded, pages.settings.draft = true, string(data)
	tester := ui.NewTester(app.standardView, 1000, 1400)
	if err := tester.Click("Settings JSON"); err != nil {
		t.Fatal(err)
	}
	tester.Compose("中文", 1)
	if !pages.settings.composing {
		t.Fatal("settings preedit missing")
	}
	if err := tester.Click("Save settings…"); err != nil {
		t.Fatal(err)
	}
	if pages.settings.confirm {
		t.Fatal("settings save enabled mid-click after IME blur")
	}
}

func TestNativeBackupSelectionDisplaysIdentityAndCancelDoesNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PI_SWITCH_CONFIG", path)
	app := &desktopShell{}
	pages := app.standardPages()
	pages.page = pageBackups
	modified := time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)
	pages.backups.loaded, pages.backups.selected = true, "config-fixture.json"
	pages.backups.items = []application.ConfigBackup{{Name: "config-fixture.json", Size: 123, ModifiedAt: modified}}
	tester := ui.NewTester(app.standardView, 560, 1200)
	if !tester.HasText("Selected snapshot: config-fixture.json · 123 bytes · modified " + modified.Local().Format("2006-01-02 15:04:05 -0700")) {
		t.Fatal("selected backup cannot be identified")
	}
	if err := tester.Click("Restore selected backup…"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Restore config backup?") {
		t.Fatal("restore lacked confirmation")
	}
	if err := tester.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if pages.backups.confirm || pages.backups.busy || pages.backups.selected != "config-fixture.json" {
		t.Fatal("cancel lost selection or started restore")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("cancel wrote config")
	}
}

func TestNativeKnownErrorsHaveSpecificHintsWithoutRawValues(t *testing.T) {
	for _, tc := range []struct {
		err  error
		hint string
	}{
		{&application.ManagementValidationError{Message: "ports must be between 1 and 65535"}, "65535"},
		{fmt.Errorf("fake-secret: %w", profile.ErrProfileExists), "already exists"},
		{&os.PathError{Op: "open", Path: "fake-secret", Err: os.ErrPermission}, "permission denied"},
	} {
		text := nativeManagementError(tc.err)
		if !strings.Contains(text, tc.hint) || strings.Contains(text, "fake-secret") {
			t.Fatalf("unsafe/unhelpful error: %s", text)
		}
	}
}

// A preedit in the saved-provider copy name renders below the draft row, so that
// row must read the composition state when the press arrives instead of the
// value captured when it was built.
func TestNativeProviderCompositionBlocksDraftNetworkButtons(t *testing.T) {
	for _, button := range []string{"Validate provider", "Test draft connection", "Fetch models into draft"} {
		t.Run(button, func(t *testing.T) {
			app, p, _ := newProviderPageFixture(t)
			p.fixtureDraft("supplier")
			requested := 0
			app.taskRequested = func(string) { requested++ }
			tester := ui.NewTester(app.standardView, 1000, 2400)
			tester.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
			if err := tester.Click("More provider actions"); err != nil {
				t.Fatal(err)
			}
			if err := tester.Click("Saved provider copy name"); err != nil {
				t.Fatal(err)
			}
			tester.Compose("中文供应商", 3)
			tester.Frame()
			if !p.composing {
				t.Fatal("composition in the copy name was not tracked")
			}
			if err := tester.Click(button); err != nil {
				t.Fatal(err)
			}
			tester.Frame()
			if requested != 0 {
				t.Fatalf("a preedit in the copy name started work through %s", button)
			}
			// Committing the preedit leaves the action usable.
			tester.Compose("", 0)
			tester.Frame()
			if err := tester.Click(button); err != nil {
				t.Fatal(err)
			}
			tester.Frame()
			if requested != 1 {
				t.Fatalf("a committed preedit did not let %s through: requested=%d", button, requested)
			}
		})
	}
}
