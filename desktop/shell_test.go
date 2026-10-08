package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
)

func TestStartsMinimized(t *testing.T) {
	if !startsMinimized([]string{"--start-minimized"}) {
		t.Fatal("--start-minimized was ignored")
	}
	if startsMinimized([]string{"--help"}) {
		t.Fatal("unexpected minimized startup")
	}
}

func TestProxyExecutableUsesConfiguredPath(t *testing.T) {
	path, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CLI_PATH", path)
	got, err := proxyExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("proxyExecutable() = %q, want %q", got, path)
	}
}

func TestProxyExecutableRejectsInvalidConfiguredPath(t *testing.T) {
	t.Setenv("PI_SWITCH_CLI_PATH", filepath.Join(t.TempDir(), "missing-pi-switch"))
	if _, err := proxyExecutable(); err == nil {
		t.Fatal("invalid configured CLI path was accepted")
	}
}

func TestStartsHiddenOnlyWhenTrayIsAvailable(t *testing.T) {
	app := &desktopShell{startMinimized: true}
	if app.startsHidden() {
		t.Fatal("window was hidden without a tray recovery path")
	}
	app.tray = &mygo.Tray{}
	if !app.startsHidden() {
		t.Fatal("minimized startup with a tray should hide the window")
	}
}

func TestMenusKeepActionsReachable(t *testing.T) {
	app := &desktopShell{}
	applicationMenu := app.applicationMenu()
	for _, label := range []string{"Show pi-switch", "Show Native Overview", "Show Complex UI Prototypes", "Start Proxy", "Stop Proxy"} {
		if !menuHasLabel(applicationMenu.Items(), label) {
			t.Fatalf("application menu is missing %q", label)
		}
	}
	if !menuHasRole(applicationMenu.Items(), mygo.RoleQuit) {
		t.Fatal("application menu is missing Quit")
	}
	trayMenu := app.trayMenu()
	for _, label := range []string{"Show pi-switch", "Show Native Overview", "Show Complex UI Prototypes", "Quit"} {
		if !menuHasLabel(trayMenu.Items(), label) {
			t.Fatalf("tray menu is missing %q", label)
		}
	}
}

func TestWindowCloseHidesUnlessTheAppIsQuitting(t *testing.T) {
	app := &desktopShell{}
	closeEvent := &mygo.CloseEvent{}
	app.onWindowClose(closeEvent)
	if !closeEvent.DefaultPrevented() {
		t.Fatal("closing the window should hide it rather than quit")
	}
	app.exiting = true
	quitEvent := &mygo.CloseEvent{}
	app.onWindowClose(quitEvent)
	if quitEvent.DefaultPrevented() {
		t.Fatal("explicit app quit should be allowed to close the window")
	}
}

func menuHasLabel(items []*mygo.MenuItem, label string) bool {
	for _, item := range items {
		if item.Label == label || menuHasLabel(item.Submenu, label) {
			return true
		}
	}
	return false
}

func menuHasRole(items []*mygo.MenuItem, role mygo.MenuRole) bool {
	for _, item := range items {
		if item.Role == role || menuHasRole(item.Submenu, role) {
			return true
		}
	}
	return false
}

func TestNativeWindowCloseHidesUnlessTheAppIsQuitting(t *testing.T) {
	app := &desktopShell{}
	closeEvent := &mygo.CloseEvent{}
	app.onNativeWindowClose(closeEvent)
	if !closeEvent.DefaultPrevented() {
		t.Fatal("closing the native overview should hide it rather than quit")
	}
	app.exiting = true
	quitEvent := &mygo.CloseEvent{}
	app.onNativeWindowClose(quitEvent)
	if quitEvent.DefaultPrevented() {
		t.Fatal("explicit app quit should be allowed to close the native overview")
	}
}

func TestComplexWindowCloseHidesUnlessTheAppIsQuitting(t *testing.T) {
	app := &desktopShell{}
	closeEvent := &mygo.CloseEvent{}
	app.onComplexWindowClose(closeEvent)
	if !closeEvent.DefaultPrevented() {
		t.Fatal("closing the complex preview should hide it rather than quit")
	}
	app.exiting = true
	quitEvent := &mygo.CloseEvent{}
	app.onComplexWindowClose(quitEvent)
	if quitEvent.DefaultPrevented() {
		t.Fatal("explicit app quit should be allowed to close the complex preview")
	}
}
