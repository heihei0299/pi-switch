package main

import (
	"errors"
	"log"
	"os"

	"github.com/egoist/mygo"
)

func main() {
	mygo.App.SetName("pi-switch")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	shell, err := prepareDesktopShell(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	mygo.App.SetMenu(shell.applicationMenu())
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		shell.showWindow()
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		shell.exiting = true
		shell.tasks.close()
	})
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

func prepareDesktopShell(args []string) (*desktopShell, error) {
	for _, arg := range args {
		if arg == "--web" {
			return nil, errors.New("--web is no longer supported; Desktop always uses the Native GUI")
		}
	}
	shell := &desktopShell{startMinimized: startsMinimized(args), proxyMessage: "Checking proxy status…"}
	shell.complexPreview = newNativeComplexPreview()
	shell.complexPreview.refreshGateway = shell.refreshNativeGatewayPreview
	shell.complexPreview.publishGateway = shell.publishNativeGatewaySelection
	shell.complexPreview.publishGatewayDraft = shell.publishNativeGatewayDraft
	shell.complexPreview.runPackageTask = shell.runNativePackageTask
	shell.complexPreview.refreshStats = shell.refreshNativeStats
	shell.complexPreview.exportStats = shell.exportNativeStats
	shell.complexPreview.refreshConversationRequests = shell.refreshNativeConversationRequests
	return shell, nil
}
