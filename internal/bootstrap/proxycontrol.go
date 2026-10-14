package bootstrap

import (
	"github.com/heihei0299/pi-switch/internal/adapters/outbound/daemon"
	"github.com/heihei0299/pi-switch/internal/usecase/proxycontrol"
)

func NewProxyControlService() proxycontrol.Service {
	return proxycontrol.New(daemon.Adapter{})
}
