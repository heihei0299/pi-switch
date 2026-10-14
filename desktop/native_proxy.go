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

func (p nativeProxyStatus) view(c *ui.Context) {
	if !p.checked {
		ui.Text(c, "Proxy identity and health: unknown (not checked).")
		return
	}
	ui.Text(c, "Last Proxy check: "+p.checkedAt)
	if p.failed {
		ui.Text(c, "Proxy identity and health: unknown (check failed).")
		return
	}
	if !p.result.Running {
		ui.Text(c, "No healthy, identity-verified managed Proxy was found.")
		return
	}
	ui.Text(c, "Managed Proxy identity and health verified.")
	if p.result.Pid != nil {
		ui.Text(c, fmt.Sprintf("Managed PID: %d", *p.result.Pid))
	}
	if p.result.Host != nil && p.result.Port != nil {
		ui.Text(c, "Actual listener: "+net.JoinHostPort(*p.result.Host, strconv.Itoa(int(*p.result.Port))))
	}
}

type nativeProxyController struct {
	status      nativeProxyStatus
	busy        bool
	message     string
	stopConfirm bool
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

func (p *nativeProxyController) controls(shell *desktopShell, c *ui.Context) {
	message := p.message
	if message == "" {
		message = "Status not checked yet."
	}
	ui.Text(c, message).Role(ui.RoleStatus)
	p.status.view(c)
	ui.Row(c).Gap(8).Children(func() {
		if ui.PrimaryButton(c, "Start Proxy").Disabled(p.busy).Clicked() {
			p.start(shell)
		}
		if ui.Button(c, "Stop Proxy").Disabled(p.busy).Clicked() {
			p.stopConfirm = true
		}
		if ui.Button(c, "Refresh Status").Disabled(p.busy).Clicked() {
			p.refresh(shell)
		}
	})
	if ui.AlertDialog(c, &p.stopConfirm, "Stop managed Proxy?", "In-flight requests may fail. Unknown processes will not be stopped.", "Cancel", "Stop") == 1 {
		p.stop(shell)
	}
}
