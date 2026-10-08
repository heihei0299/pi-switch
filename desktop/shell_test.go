package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
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
	for _, label := range []string{"Show pi-switch", "Start Proxy", "Stop Proxy"} {
		if !menuHasLabel(applicationMenu.Items(), label) {
			t.Fatalf("application menu is missing %q", label)
		}
	}
	if !menuHasRole(applicationMenu.Items(), mygo.RoleQuit) {
		t.Fatal("application menu is missing Quit")
	}
	if !menuHasLabel(app.trayMenu().Items(), "Quit") {
		t.Fatal("tray menu is missing Quit")
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

func TestDesktopShellViewShowsFallbackAndAcceptsInput(t *testing.T) {
	app := &desktopShell{
		proxyMessage: "Proxy daemon is not running",
		trayMessage:  "System tray unavailable; use File > Show pi-switch.",
	}
	tester := ui.NewTester(app.view, 760, 560)
	if !tester.HasText("System tray unavailable") {
		t.Fatalf("tray fallback message missing from %q", tester.Texts())
	}
	if err := tester.Click("IME / keyboard input"); err != nil {
		t.Fatal(err)
	}
	tester.Type("中文")
	if !tester.HasText("中文") {
		t.Fatalf("typed text not reflected in UI: %q", tester.Texts())
	}
}
