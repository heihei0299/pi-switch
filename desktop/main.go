package main

import (
	"log"
	"os"

	"github.com/egoist/mygo"
)

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
