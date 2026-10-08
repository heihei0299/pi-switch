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
	"github.com/heihei0299/pi-switch/internal/daemon"
)

const (
	proxyHost = "127.0.0.1"
	proxyPort = 43112
)

type desktopShell struct {
	window         *mygo.Window
	tray           *mygo.Tray
	startMinimized bool
	exiting        bool
	busy           bool
	input          string
	proxyMessage   string
	trayMessage    string
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

func (a *desktopShell) proxyService() (daemon.Service, error) {
	executable, err := proxyExecutable()
	if err != nil {
		return daemon.Service{}, err
	}
	service := daemon.Proxy
	service.Executable = executable
	return service, nil
}

func (a *desktopShell) startProxy() {
	a.runProxyTask(func() (daemon.DaemonResult, error) {
		status, err := daemon.Status(daemon.Proxy)
		if err != nil || status.Running {
			return status, err
		}
		service, err := a.proxyService()
		if err != nil {
			return daemon.DaemonResult{}, err
		}
		return daemon.Start(service, proxyHost, proxyPort)
	})
}

func (a *desktopShell) stopProxy() {
	a.runProxyTask(func() (daemon.DaemonResult, error) {
		return daemon.Stop(daemon.Proxy)
	})
}

func (a *desktopShell) refreshProxy() {
	a.runProxyTask(func() (daemon.DaemonResult, error) {
		return daemon.Status(daemon.Proxy)
	})
}

func (a *desktopShell) runProxyTask(run func() (daemon.DaemonResult, error)) {
	if a.busy || a.exiting || a.window == nil {
		return
	}
	a.busy = true
	a.window.Invalidate()
	window := a.window
	go func() {
		result, err := run()
		message := result.Message
		if err != nil {
			message = err.Error()
		}
		window.Update(func() {
			a.busy = false
			a.proxyMessage = message
		})
	}()
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
			{Label: "Refresh Proxy Status", Click: func(*mygo.MenuItem, *mygo.Window) { a.refreshProxy() }},
			{Label: "Start Proxy", Click: func(*mygo.MenuItem, *mygo.Window) { a.startProxy() }},
			{Label: "Stop Proxy", Click: func(*mygo.MenuItem, *mygo.Window) { a.stopProxy() }},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Role: mygo.RoleEditMenu},
	})
}

func (a *desktopShell) trayMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "Show pi-switch", Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
		{Label: "Refresh Proxy Status", Click: func(*mygo.MenuItem, *mygo.Window) { a.refreshProxy() }},
		{Label: "Start Proxy", Click: func(*mygo.MenuItem, *mygo.Window) { a.startProxy() }},
		{Label: "Stop Proxy", Click: func(*mygo.MenuItem, *mygo.Window) { a.stopProxy() }},
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
	a.trayMessage = "System tray unavailable; use File > Show pi-switch (Alt+F10 on Linux/Windows). " + err.Error()
	log.Printf("desktop tray unavailable: %v", err)
}

func (a *desktopShell) startsHidden() bool {
	return a.startMinimized && a.tray != nil
}

func (a *desktopShell) start() {
	a.createTray()
	hidden := a.startsHidden()
	a.window = mygo.NewWindow(mygo.WindowOptions{
		Title:           "pi-switch — Desktop (development)",
		Width:           760,
		Height:          560,
		MinWidth:        560,
		MinHeight:       420,
		BackgroundColor: "light-dark(#f6f7f9, #0f1115)",
		StateKey:        "main",
		Hidden:          hidden,
		AutoHideMenuBar: true,
		Content:         ui.View(a.view),
	})
	a.window.OnClose(a.onWindowClose)
	a.refreshProxy()
}

func (a *desktopShell) onWindowClose(e *mygo.CloseEvent) {
	if a.exiting {
		return
	}
	e.PreventDefault()
	if a.window != nil {
		a.window.Hide()
	}
}

func (a *desktopShell) view(c *ui.Context) {
	theme := c.Theme()
	ui.Column(c).Fill().Padding(24).Gap(14).Children(func() {
		ui.Text(c, "pi-switch").FontSize(26).Bold()
		ui.Text(c, "Desktop shell · closing this window keeps the proxy running.").TextColor(theme.TextMuted)
		ui.Text(c, "Proxy status").FontSize(18).Bold()
		message := a.proxyMessage
		if message == "" {
			message = "Status not checked yet."
		}
		ui.Text(c, message).TextColor(theme.Text)
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "Start Proxy").Disabled(a.busy).Clicked() {
				a.startProxy()
			}
			if ui.Button(c, "Stop Proxy").Disabled(a.busy).Clicked() {
				a.stopProxy()
			}
			if ui.Button(c, "Refresh Status").Disabled(a.busy).Clicked() {
				a.refreshProxy()
			}
		})
		if a.trayMessage != "" {
			ui.Text(c, a.trayMessage).TextColor(theme.Warning)
		}
		ui.Text(c, "Input check").FontSize(18).Bold()
		ui.TextInput(c, &a.input).Label("IME / keyboard input").Placeholder("Type or paste here").Width(480)
		ui.Text(c, a.input).TextColor(theme.TextMuted)
	})
}
