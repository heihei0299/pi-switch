package main

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/bootstrap"
	"github.com/heihei0299/pi-switch/internal/usecase/proxycontrol"
)

type nativeProxyStatus struct {
	result          proxycontrol.Result
	checked, failed bool
	checkedAt       string
}

// nativeProxyAction is the one operation a verified status allows.
type nativeProxyAction int

const (
	proxyActionCheck nativeProxyAction = iota
	proxyActionStart
	proxyActionStop
)

// action reports what the last check allows. A check that never ran or failed
// leaves the status unknown, which offers another check: an unknown Proxy must
// never be presented as stopped.
func (s nativeProxyStatus) action() nativeProxyAction {
	if !s.checked || s.failed {
		return proxyActionCheck
	}
	if s.result.Running {
		return proxyActionStop
	}
	return proxyActionStart
}

// summary states only what has been verified; unknown is never reported as
// running or stopped.
func (s nativeProxyStatus) summary() string {
	switch s.action() {
	case proxyActionStop:
		return "Managed Proxy identity and health verified."
	case proxyActionStart:
		return "No healthy, identity-verified managed Proxy was found."
	default:
		if !s.checked {
			return "Proxy identity and health: unknown (not checked)."
		}
		return "Proxy identity and health: unknown (check failed)."
	}
}

// details renders verification evidence, which stays out of the main area.
func (s nativeProxyStatus) details(c *ui.Context) {
	if !s.checked {
		ui.Text(c, "No Proxy check has run yet.")
		return
	}
	ui.Text(c, "Last Proxy check: "+s.checkedAt)
	if s.failed {
		ui.Text(c, "The last check did not complete; the status stays unknown.")
	}
	if s.result.Pid != nil {
		ui.Text(c, fmt.Sprintf("Managed PID: %d", *s.result.Pid))
	}
	if s.result.Host != nil && s.result.Port != nil {
		ui.Text(c, "Actual listener: "+net.JoinHostPort(*s.result.Host, strconv.Itoa(int(*s.result.Port))))
	}
}

type nativeProxyController struct {
	status      nativeProxyStatus
	busy        bool
	message     string
	stopConfirm bool
	detailsOpen bool
}

func (p *nativeProxyController) service() (proxycontrol.Target, error) {
	executable, err := proxyExecutable()
	if err != nil {
		return proxycontrol.Target{}, err
	}
	return proxycontrol.Target{Name: proxycontrol.Proxy.Name, Executable: executable}, nil
}

func (p *nativeProxyController) start(shell *desktopShell) {
	p.runTask(shell, func() (proxycontrol.Result, error) {
		control := bootstrap.NewProxyControlService()
		status, err := control.Status(proxycontrol.Proxy)
		if err != nil || status.Running {
			return status, err
		}
		host, port, err := desktopProxyAddress()
		if err != nil {
			return proxycontrol.Result{}, err
		}
		service, err := p.service()
		if err != nil {
			return proxycontrol.Result{}, err
		}
		return control.Start(service, host, port)
	})
}

func (p *nativeProxyController) stop(shell *desktopShell) {
	p.runTask(shell, func() (proxycontrol.Result, error) {
		return bootstrap.NewProxyControlService().Stop(proxycontrol.Proxy)
	})
}

func (p *nativeProxyController) refresh(shell *desktopShell) {
	p.runTask(shell, func() (proxycontrol.Result, error) {
		return bootstrap.NewProxyControlService().Status(proxycontrol.Proxy)
	})
}

func (p *nativeProxyController) runTask(shell *desktopShell, run func() (proxycontrol.Result, error)) {
	if p.busy || shell.exiting || shell.window == nil || shell.window.IsDestroyed() {
		return
	}
	window := shell.window
	ctx, token, ok := shell.tasks.begin("proxy")
	if !ok {
		return
	}
	p.busy = true
	window.Invalidate()
	go func() {
		result, err := run()
		if ctx.Err() != nil {
			return
		}
		message := result.Message
		if err != nil {
			message = nativeManagementError(err)
		}
		checkedAt := time.Now().Format(time.RFC3339)
		window.Update(func() {
			if window.IsDestroyed() || !shell.tasks.accept("proxy", token) {
				shell.tasks.finish("proxy", token)
				return
			}
			defer shell.tasks.finish("proxy", token)
			p.busy = false
			p.message = message
			p.status = nativeProxyStatus{result: result, checked: true, failed: err != nil, checkedAt: checkedAt}
			if shell.window != nil {
				shell.window.Invalidate()
			}
		})
	}()
}

// statusMessage keeps the last daemon message, including identity and port
// failures, in the main area rather than behind the details disclosure.
func (p *nativeProxyController) statusMessage() string {
	if p.message != "" {
		return p.message
	}
	if !p.status.checked {
		return "Status not checked yet."
	}
	return ""
}

func (p *nativeProxyController) controls(shell *desktopShell, c *ui.Context) {
	ui.Text(c, p.status.summary())
	if message := p.statusMessage(); message != "" {
		ui.Text(c, message).Role(ui.RoleStatus)
	}
	ui.Row(c).Gap(8).Children(func() {
		switch p.status.action() {
		case proxyActionCheck:
			if ui.PrimaryButton(c, "Check Proxy Status").Disabled(p.busy).Clicked() {
				p.refresh(shell)
			}
		case proxyActionStart:
			if ui.PrimaryButton(c, "Start Proxy").Disabled(p.busy).Clicked() {
				p.start(shell)
			}
			if ui.Button(c, "Refresh Status").Disabled(p.busy).Clicked() {
				p.refresh(shell)
			}
		case proxyActionStop:
			if ui.PrimaryButton(c, "Stop Proxy").Disabled(p.busy).Clicked() {
				p.stopConfirm = true
			}
			if ui.Button(c, "Refresh Status").Disabled(p.busy).Clicked() {
				p.refresh(shell)
			}
		}
	})
	ui.Collapsible(c, "Proxy details", &p.detailsOpen, func() {
		p.status.details(c)
	})
	if ui.AlertDialog(c, &p.stopConfirm, "Stop managed Proxy?", "In-flight requests may fail. Unknown processes will not be stopped.", "Cancel", "Stop") == 1 {
		p.stop(shell)
	}
}
