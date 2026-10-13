package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/config"
)

type desktopShell struct {
	window         *mygo.Window
	tray           *mygo.Tray
	startMinimized bool
	exiting        bool
	trayMessage    string
	overview       nativeOverviewController
	standard       *nativeStandardPages
	proxy          nativeProxyController
	complexPreview *nativeComplexPreview
	tasks          desktopTasks
}

func startsMinimized(args []string) bool {
	for _, arg := range args {
		if arg == "--start-minimized" {
			return true
		}
	}
	return false
}

// ponytail: CLI discovery uses resources, PATH or PI_SWITCH_CLI_PATH until WP-07 bundles target binaries.
func proxyExecutable() (string, error) {
	name := "pi-switch"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if path := os.Getenv("PI_SWITCH_CLI_PATH"); path != "" {
		return executablePath(path)
	}
	if resources, err := mygo.App.Path(mygo.PathResources); err == nil {
		if path, err := executablePath(filepath.Join(resources, name)); err == nil {
			return path, nil
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return filepath.Abs(path)
	}
	return "", fmt.Errorf("pi-switch CLI not found; bundle it beside the desktop app or set PI_SWITCH_CLI_PATH")
}

func executablePath(path string) (string, error) {
	found, err := exec.LookPath(path)
	if err != nil {
		return "", fmt.Errorf("pi-switch CLI %q is unavailable: %w", path, err)
	}
	return filepath.Abs(found)
}

func desktopProxyAddress() (string, uint16, error) {
	cfg, _, err := config.LoadConfigAtPath(config.ResolvePath())
	if err != nil {
		return "", 0, err
	}
	port := cfg.Settings.Proxy.Port
	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("proxy port %d is outside the valid range", port)
	}
	return cfg.Settings.Proxy.Host, uint16(port), nil
}

func (a *desktopShell) showWindow() {
	if a.window == nil {
		return
	}
	a.window.Restore()
	a.window.Show()
	a.window.Focus()
}

func (a *desktopShell) applicationMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "File", Submenu: []*mygo.MenuItem{
			{Label: "Show pi-switch", Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
			{Label: "Refresh Proxy Status", Click: func(*mygo.MenuItem, *mygo.Window) { a.proxy.refresh(a) }},
			{Label: "Start Proxy", Click: func(*mygo.MenuItem, *mygo.Window) { a.proxy.start(a) }},
			{Label: "Stop Proxy", Click: func(*mygo.MenuItem, *mygo.Window) { a.proxy.stop(a) }},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Role: mygo.RoleEditMenu},
	})
}

func (a *desktopShell) trayMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "Show pi-switch", Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
		{Label: "Refresh Proxy Status", Click: func(*mygo.MenuItem, *mygo.Window) { a.proxy.refresh(a) }},
		{Label: "Start Proxy", Click: func(*mygo.MenuItem, *mygo.Window) { a.proxy.start(a) }},
		{Label: "Stop Proxy", Click: func(*mygo.MenuItem, *mygo.Window) { a.proxy.stop(a) }},
		mygo.Separator(),
		{Label: "Quit", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
}

func (a *desktopShell) createTray() {
	resources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		a.setTrayError(err)
		return
	}
	icon, err := os.ReadFile(filepath.Join(resources, "icon.png"))
	if err != nil {
		a.setTrayError(err)
		return
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:    icon,
		ToolTip: "pi-switch",
		Menu:    a.trayMenu(),
	})
	if err != nil {
		a.setTrayError(err)
		return
	}
	a.tray = tray
}

func (a *desktopShell) setTrayError(err error) {
	a.trayMessage = "System tray unavailable; use File > Show pi-switch (Alt+F10 on Linux). " + err.Error()
	log.Printf("desktop tray unavailable: %v", err)
}

func (a *desktopShell) startsHidden() bool {
	return a.startMinimized && a.tray != nil
}

func (a *desktopShell) primaryWindowOptions() mygo.WindowOptions {
	return a.windowOptions(a.startsHidden())
}

func (a *desktopShell) windowOptions(hidden bool) mygo.WindowOptions {
	return mygo.WindowOptions{Title: "pi-switch — Native Management", Width: 1000, Height: 760, MinWidth: 560, MinHeight: 420, BackgroundColor: "light-dark(#f6f7f9, #0f1115)", StateKey: "native-overview", Hidden: hidden, AutoHideMenuBar: true, Content: ui.View(a.standardView)}
}

func (a *desktopShell) start() {
	a.createTray()
	a.window = mygo.NewWindow(a.primaryWindowOptions())
	a.window.OnClose(func(e *mygo.CloseEvent) {
		if a.onWindowClose(e) {
			mygo.App.Quit()
		}
	})
	a.refreshNativeOverview()
	a.proxy.refresh(a)
}

func (a *desktopShell) refreshNativeOverview() { a.overview.refresh(a) }

func (a *desktopShell) switchProfile(name string) { a.overview.switchProfile(a, name) }

func (a *desktopShell) onWindowClose(e *mygo.CloseEvent) bool {
	if a.exiting {
		return false
	}
	if a.tray == nil {
		return true
	}
	e.PreventDefault()
	if a.window != nil {
		a.window.Hide()
	}
	return false
}

func (a *desktopShell) view(c *ui.Context) { a.overview.view(a, c) }
