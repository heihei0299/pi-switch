package main

import (
	"fmt"
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

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

func main() {
	mygo.App.SetName("pi-switch")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	uiURL, err := registerManagementUI()
	if err != nil {
		log.Fatal(err)
	}

	shell := &desktopShell{
		startMinimized: startsMinimized(os.Args[1:]),
		proxyMessage:   "Checking proxy status…",
		uiURL:          uiURL,
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
