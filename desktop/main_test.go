package main

import (
	"testing"

	"github.com/egoist/mygo"
)

func TestDefaultStartupUsesNativeUI(t *testing.T) {
	app, err := prepareDesktopShell(nil)
	if err != nil || app == nil || app.startMinimized {
		t.Fatalf("Native startup failed: app=%+v err=%v", app, err)
	}
	if app.complexPreview == nil || app.complexPreview.refreshGateway == nil || app.complexPreview.refreshStats == nil {
		t.Fatal("Native feature pages were not attached to the main shell")
	}
	opts := app.primaryWindowOptions()
	if opts.Content == nil || opts.URL != "" || opts.Hidden {
		t.Fatal("Native startup loaded a URL or hid without tray recovery")
	}
}

func TestStartMinimizedStartupRemainsSupported(t *testing.T) {
	app, err := prepareDesktopShell([]string{"--start-minimized"})
	if err != nil || app == nil || !app.startMinimized {
		t.Fatalf("minimized Native startup failed: app=%+v err=%v", app, err)
	}
	app.tray = &mygo.Tray{}
	if !app.primaryWindowOptions().Hidden {
		t.Fatal("Native minimized startup ignored available tray")
	}
}

func TestNativeCompatibilityFlagRemainsSupported(t *testing.T) {
	app, err := prepareDesktopShell([]string{"--native"})
	if err != nil || app == nil || app.startMinimized {
		t.Fatalf("historical --native startup failed: app=%+v err=%v", app, err)
	}
}

func TestWebFlagIsRejectedBeforeSingleInstanceLock(t *testing.T) {
	for _, lockGranted := range []bool{false, true} {
		lockRequested := false
		gotLock, err := validateDesktopArgsBeforeLock([]string{"--web"}, func() bool {
			lockRequested = true
			return lockGranted
		})
		if err == nil || err.Error() != "--web is no longer supported; Desktop always uses the Native GUI" {
			t.Fatalf("--web with lockGranted=%t error = %v", lockGranted, err)
		}
		if lockRequested || gotLock {
			t.Fatalf("--web with lockGranted=%t reached single-instance lock", lockGranted)
		}
	}
}

func TestPrepareDesktopShellRejectsWebFlag(t *testing.T) {
	if app, err := prepareDesktopShell([]string{"--web"}); app != nil || err == nil {
		t.Fatal("retired WebView flag did not fail clearly")
	}
}

func TestSecondInstanceStillYieldsToExistingInstance(t *testing.T) {
	lockRequested := false
	gotLock, err := validateDesktopArgsBeforeLock(nil, func() bool {
		lockRequested = true
		return false
	})
	if err != nil || gotLock || !lockRequested {
		t.Fatalf("existing instance handling changed: gotLock=%t requested=%t err=%v", gotLock, lockRequested, err)
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
