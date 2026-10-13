package main

import (
	"errors"
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/heihei0299/pi-switch/internal/gateway"
)

func main() {
	mygo.App.SetName("pi-switch")
	locked, err := validateDesktopArgsBeforeLock(os.Args[1:], mygo.App.RequestSingleInstanceLock)
	if err != nil {
		log.Fatal(err)
	}
	if !locked {
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

func validateDesktopArgsBeforeLock(args []string, requestLock func() bool) (bool, error) {
	if err := validateDesktopArgs(args); err != nil {
		return false, err
	}
	return requestLock(), nil
}

func validateDesktopArgs(args []string) error {
	for _, arg := range args {
		if arg == "--web" {
			return errors.New("--web is no longer supported; Desktop always uses the Native GUI")
		}
	}
	return nil
}

func prepareDesktopShell(args []string) (*desktopShell, error) {
	if err := validateDesktopArgs(args); err != nil {
		return nil, err
	}
	shell := &desktopShell{startMinimized: startsMinimized(args), proxy: nativeProxyController{message: "Checking proxy status…"}}
	shell.complexPreview = newNativeComplexPreview()
	complex := shell.complexPreview
	gatewayState := &complex.nativeGatewayState
	gatewayState.refreshGateway = func(selections []gateway.GatewaySelection) { gatewayState.refresh(shell, selections) }
	gatewayState.publishGateway = func(selections []gateway.GatewaySelection) { gatewayState.publishSelection(shell, selections) }
	complex.nativeJSONState.publishGatewayDraft = func(raw string) { gatewayState.publishDraft(shell, &complex.nativeJSONState, raw) }
	complex.nativePackagesState.runPackageTask = func(task func() (string, error)) { complex.nativePackagesState.runTask(shell, task) }
	complex.nativeStatsState.refreshStats = func(requestPage, conversationPage int, statsFilters nativeStatsFilters, sessionsFilters nativeSessionsFilters) {
		complex.nativeStatsState.refresh(shell, &complex.nativeSessionsState, requestPage, conversationPage, statsFilters, sessionsFilters)
	}
	complex.nativeStatsState.exportStats = func(format string) { complex.nativeStatsState.export(shell, format) }
	complex.nativeSessionsState.refreshConversationRequests = func(id string, page int) {
		complex.nativeSessionsState.refreshRequests(shell, id, page)
	}
	return shell, nil
}
