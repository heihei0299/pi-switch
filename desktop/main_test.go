package main

import (
	"testing"

	"github.com/egoist/mygo"
)

func TestDefaultStartupUsesNativeUI(t *testing.T) {
	app, err := prepareDesktopShell([]string{"--start-minimized"})
	if err != nil || app == nil || !app.startMinimized {
		t.Fatalf("Native startup failed: app=%+v err=%v", app, err)
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

func TestWebFlagIsRejected(t *testing.T) {
	if app, err := prepareDesktopShell([]string{"--web"}); app != nil || err == nil {
		t.Fatal("retired WebView flag did not fail clearly")
	}
}

func TestNativePrimaryCloseUsesMainWindowPolicy(t *testing.T) {
	app := &desktopShell{}
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
