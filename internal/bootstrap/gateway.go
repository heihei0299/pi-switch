package bootstrap

import (
	"github.com/heihei0299/pi-switch/internal/adapters/outbound/catalog"
	"github.com/heihei0299/pi-switch/internal/adapters/outbound/filegateway"
	gatewayusecase "github.com/heihei0299/pi-switch/internal/usecase/gateway"
)

func NewGatewayService() *gatewayusecase.Service {
	return gatewayusecase.New(filegateway.New(filegateway.ModelsPath()), catalog.New())
}
