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
	shell, err := prepareDesktopShell(os.Args[1:], registerManagementUI)
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

func prepareDesktopShell(args []string, registerUI func() (string, error)) (*desktopShell, error) {
	shell := &desktopShell{nativeOnly: startsNative(args), startMinimized: startsMinimized(args), proxyMessage: "Checking proxy status…"}
	if !shell.nativeOnly {
		url, err := registerUI()
		if err != nil {
			return nil, err
		}
		shell.uiURL = url
	} else {
		shell.complexPreview = newNativeComplexPreview()
		shell.complexPreview.refreshGateway = shell.refreshNativeGatewayPreview
		shell.complexPreview.publishGateway = shell.publishNativeGatewaySelection
		shell.complexPreview.publishGatewayDraft = shell.publishNativeGatewayDraft
		shell.complexPreview.runPackageTask = shell.runNativePackageTask
		shell.complexPreview.refreshStats = shell.refreshNativeStats
		shell.complexPreview.exportStats = shell.exportNativeStats
		shell.complexPreview.refreshConversationRequests = shell.refreshNativeConversationRequests
	}
	return shell, nil
}
