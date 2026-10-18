package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/usecase/proxycontrol"
)

func TestNativeProxyActionFollowsVerifiedStatus(t *testing.T) {
	pid := uint32(4242)
	for _, tc := range []struct {
		name   string
		status nativeProxyStatus
		want   nativeProxyAction
	}{
		{"not checked", nativeProxyStatus{}, proxyActionCheck},
		{"check failed", nativeProxyStatus{checked: true, failed: true}, proxyActionCheck},
		{"failed check keeps the status unknown", nativeProxyStatus{checked: true, failed: true, result: proxycontrol.Result{Running: true, Pid: &pid}}, proxyActionCheck},
		{"checked without a managed proxy", nativeProxyStatus{checked: true, result: proxycontrol.Result{Running: false, Pid: &pid}}, proxyActionStart},
		{"verified running proxy", nativeProxyStatus{checked: true, result: proxycontrol.Result{Running: true, Pid: &pid}}, proxyActionStop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.status.action(); got != tc.want {
				t.Fatalf("action = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNativeProxyUnknownStatusOffersOnlyACheck(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status nativeProxyStatus
	}{
		{"not checked", nativeProxyStatus{}},
		{"check failed", nativeProxyStatus{checked: true, failed: true, checkedAt: "2026-10-18T10:00:00Z"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := &desktopShell{proxy: nativeProxyController{status: tc.status}}
			tester := ui.NewTester(app.view, 900, 1000)
			if !tester.HasText("Check Proxy Status") {
				t.Fatalf("unknown status offers no check: %q", tester.Texts())
			}
			if tester.HasText("Start Proxy") || tester.HasText("Stop Proxy") {
				t.Fatalf("unknown status offered start or stop: %q", tester.Texts())
			}
			if !tester.HasText("Proxy identity and health: unknown") {
				t.Fatalf("unknown status is not reported as unknown: %q", tester.Texts())
			}
		})
	}
}

func TestNativeProxyOffersOneApplicableAction(t *testing.T) {
	pid := uint32(4242)
	running := &desktopShell{proxy: nativeProxyController{status: nativeProxyStatus{
		checked: true, result: proxycontrol.Result{Running: true, Pid: &pid},
	}}}
	tester := ui.NewTester(running.view, 900, 1000)
	if !tester.HasText("Stop Proxy") || tester.HasText("Start Proxy") {
		t.Fatalf("running proxy did not offer exactly Stop: %q", tester.Texts())
	}
	if !tester.HasText("Refresh Status") || !tester.HasText("Managed Proxy identity and health verified.") {
		t.Fatalf("running proxy lost its secondary refresh or verified state: %q", tester.Texts())
	}

	stopped := &desktopShell{proxy: nativeProxyController{status: nativeProxyStatus{checked: true, result: proxycontrol.Result{Running: false}}}}
	tester = ui.NewTester(stopped.view, 900, 1000)
	if !tester.HasText("Start Proxy") || tester.HasText("Stop Proxy") {
		t.Fatalf("checked stopped proxy did not offer exactly Start: %q", tester.Texts())
	}
	if !tester.HasText("No healthy, identity-verified managed Proxy was found.") {
		t.Fatalf("checked stopped proxy lost its state line: %q", tester.Texts())
	}
}

func TestNativeProxyStatusDetailsStayCollapsedUntilRequested(t *testing.T) {
	pid := uint32(4242)
	host, port := "127.0.0.1", uint16(43112)
	app := &desktopShell{proxy: nativeProxyController{status: nativeProxyStatus{
		checked: true, checkedAt: "2026-10-18T10:00:00Z",
		result: proxycontrol.Result{Running: true, Pid: &pid, Host: &host, Port: &port},
	}}}
	tester := ui.NewTester(app.view, 900, 1000)
	if !tester.HasText("Stop Proxy") {
		t.Fatalf("verified proxy controls missing: %q", tester.Texts())
	}
	if tester.HasText("Managed PID: 4242") {
		t.Fatalf("Proxy details are not collapsed: %q", tester.Texts())
	}
	if err := tester.Click("Proxy details"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("Last Proxy check: 2026-10-18T10:00:00Z") || !tester.HasText("Managed PID: 4242") || !tester.HasText("Actual listener: 127.0.0.1:43112") {
		t.Fatalf("Proxy details missing after expanding: %q", tester.Texts())
	}
}

func TestNativeProxyIdentityMismatchMessageStaysVisible(t *testing.T) {
	pid := uint32(77)
	message := "proxy daemon PID 77 identity mismatch; unmanaged listener state cleaned up"
	app := &desktopShell{proxy: nativeProxyController{
		message: message,
		status:  nativeProxyStatus{checked: true, result: proxycontrol.Result{Running: false, Pid: &pid}},
	}}
	tester := ui.NewTester(app.view, 900, 1000)
	if !tester.HasText(message) {
		t.Fatalf("identity mismatch message is hidden: %q", tester.Texts())
	}
}
