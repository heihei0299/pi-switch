package main

import (
	"fmt"
	"net"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/daemon"
)

type nativeProxyStatus struct {
	result          daemon.DaemonResult
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
