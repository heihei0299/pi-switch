package main

import (
	"errors"
	"testing"

	"github.com/egoist/mygo"
)

func TestDefaultStartupUsesNativeWithoutRegisteringLegacyUI(t *testing.T) {
	calls := 0
	register := func() (string, error) { calls++; return "", errors.New("legacy UI deliberately unavailable") }
	app, err := prepareDesktopShell([]string{"--start-minimized"}, register)
	if err != nil || calls != 0 || app == nil || !app.nativeOnly || !app.startMinimized || app.uiURL != "" {
		t.Fatalf("Native required legacy initialization: app=%+v calls=%d err=%v", app, calls, err)
	}
	if app.complexPreview == nil || app.complexPreview.refreshGateway == nil || app.complexPreview.refreshStats == nil {
		t.Fatal("Native feature pages were not attached to the main shell")
	}
	opts := app.primaryWindowOptions()
	if opts.Content == nil || opts.URL != "" || opts.Hidden {
		t.Fatal("Native startup loaded a URL or hid without tray recovery")
	}
	app.tray = &mygo.Tray{}
	if !app.primaryWindowOptions().Hidden {
		t.Fatal("Native minimized startup ignored available tray")
	}
}

func TestLegacyWebViewRequiresExplicitRollbackFlag(t *testing.T) {
	calls := 0
	register := func() (string, error) { calls++; return "pi-switch-ui://fixture/", nil }
	app, err := prepareDesktopShell([]string{"--web"}, register)
	if err != nil || calls != 1 || app.nativeOnly {
		t.Fatal("explicit WebView fallback did not register the legacy UI")
	}
	opts := app.primaryWindowOptions()
	if opts.Content != nil || opts.URL != "pi-switch-ui://fixture/" {
		t.Fatal("WebView fallback lost its management URL")
	}
	expected := errors.New("registration failed")
	if app, err := prepareDesktopShell([]string{"--web"}, func() (string, error) { return "", expected }); app != nil || !errors.Is(err, expected) {
		t.Fatal("WebView fallback registration failure was hidden")
	}
}

func TestNativePrimaryCloseUsesMainWindowPolicy(t *testing.T) {
	app := &desktopShell{nativeOnly: true}
	event := &mygo.CloseEvent{}
	if !app.onWindowClose(event) || event.DefaultPrevented() {
		t.Fatal("Native primary window hid without a tray recovery path")
	}
	app.tray = &mygo.Tray{}
	event = &mygo.CloseEvent{}
	if app.onWindowClose(event) || !event.DefaultPrevented() {
		t.Fatal("Native primary with tray did not preserve hide policy")
	}
	app.exiting = true
	event = &mygo.CloseEvent{}
	if app.onWindowClose(event) || event.DefaultPrevented() {
		t.Fatal("Native explicit quit was intercepted")
	}
}
