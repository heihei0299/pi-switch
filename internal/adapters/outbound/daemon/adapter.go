package daemon

import (
	"fmt"

	"github.com/heihei0299/pi-switch/internal/usecase/proxycontrol"
)

type Adapter struct{}

func (Adapter) Start(target proxycontrol.Target, host string, port uint16) (proxycontrol.Result, error) {
	service, err := serviceForTarget(target)
	if err != nil {
		return proxycontrol.Result{}, err
	}
	result, err := Start(service, host, port)
	return proxyControlResult(result), err
}

func (Adapter) Stop(target proxycontrol.Target) (proxycontrol.Result, error) {
	service, err := serviceForTarget(target)
	if err != nil {
		return proxycontrol.Result{}, err
	}
	result, err := Stop(service)
	return proxyControlResult(result), err
}

func (Adapter) Status(target proxycontrol.Target) (proxycontrol.Result, error) {
	service, err := serviceForTarget(target)
	if err != nil {
		return proxycontrol.Result{}, err
	}
	result, err := Status(service)
	return proxyControlResult(result), err
}

func serviceForTarget(target proxycontrol.Target) (Service, error) {
	service := ServiceByName(target.Name)
	if service == nil {
		return Service{}, fmt.Errorf("unknown daemon target %q", target.Name)
	}
	resolved := *service
	if target.Executable != "" {
		resolved.Executable = target.Executable
	}
	return resolved, nil
}

func proxyControlResult(result DaemonResult) proxycontrol.Result {
	return proxycontrol.Result{
		Running: result.Running, Pid: result.Pid, Host: result.Host, Port: result.Port,
		Targets: result.Targets, Failover: result.Failover, StartedAt: result.StartedAt, Message: result.Message,
	}
}

var _ proxycontrol.Daemon = Adapter{}
