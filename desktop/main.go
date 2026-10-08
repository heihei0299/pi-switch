package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Greeter is callable from the frontend: `mygo generate` turns its methods
// into typed TypeScript functions in src/mygo.ts.
type Greeter struct{}

// Greet returns a greeting for name.
func (Greeter) Greet(name string) string {
	if name == "" {
		name = "stranger"
	}
	return "Hello, " + name + "! This message comes from Go."
}

// SystemInfo describes the machine the app runs on.
type SystemInfo struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"goVersion"`
}

// Info returns information about the system.
func (Greeter) Info() SystemInfo {
	return SystemInfo{OS: runtime.GOOS, Arch: runtime.GOARCH, GoVersion: runtime.Version()}
}

// Tick is sent to the page every second.
var Tick = mygo.NewEvent[time.Time]("tick")

type nativeDemo struct {
	name  string
	count int
}

func (a *nativeDemo) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(24).Gap(12).Children(func() {
		ui.Text(c, "Native controls").FontSize(24).Bold()
		ui.Text(c, "Type with fcitx5 / Rime").TextColor(t.TextMuted)
		ui.TextInput(c, &a.name).Label("Name").Placeholder("中文输入 / type here").Width(440)
		greeting := "Hello!"
		if a.name != "" {
			greeting = fmt.Sprintf("Hello, %s!", a.name)
		}
		ui.Text(c, greeting).FontSize(18)
		ui.Row(c).Gap(10).Children(func() {
			if ui.Button(c, "−").Width(44).Clicked() {
				a.count--
			}
			ui.Textf(c, "%d", a.count).FontSize(20).Width(48).TextAlign(ui.Center)
			if ui.PrimaryButton(c, "+").Width(44).Clicked() {
				a.count++
			}
		})
	})
}

func registerLegacyWebUI() error {
	dist := os.Getenv("PI_SWITCH_SPIKE_WEBUI_DIST")
	if dist == "" {
		return fmt.Errorf("PI_SWITCH_SPIKE_WEBUI_DIST must point to a built WebUI directory")
	}
	if _, err := os.Stat(filepath.Join(dist, "index.html")); err != nil {
		return fmt.Errorf("WebUI entrypoint in %q: %w", dist, err)
	}
	return mygo.Protocol.Handle("pi-switch-ui", mygo.FileServer(os.DirFS(dist)))
}

func main() {
	mygo.Bind(Greeter{})
	mygo.App.SetName("pi-switch MyGo Spike")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}

	shell := &desktopShell{
		startMinimized: startsMinimized(os.Args[1:]),
		proxyMessage:   "Checking proxy status…",
	}
	mygo.App.SetMenu(shell.applicationMenu())
	mygo.App.OnSecondInstance(func(_ []string, _ string) { shell.showWindow() })
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { shell.exiting = true })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if shell.tray != nil {
			shell.tray.Destroy()
		}
	})
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if !hasVisibleWindows {
			shell.showWindow()
		}
	})
	mygo.App.WhenReady(shell.start)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
