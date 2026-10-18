package bootstrap

import (
	"github.com/heihei0299/pi-switch/internal/adapters/outbound/catalog"
	"github.com/heihei0299/pi-switch/internal/adapters/outbound/fileconfig"
	"github.com/heihei0299/pi-switch/internal/adapters/outbound/filegateway"
	"github.com/heihei0299/pi-switch/internal/config"
	gatewayusecase "github.com/heihei0299/pi-switch/internal/usecase/gateway"
)

func NewGatewayService() *gatewayusecase.Service {
	return gatewayusecase.New(fileconfig.New(config.ResolvePath(), config.DefaultConfig), filegateway.New(filegateway.ModelsPath()), catalog.New())
}
