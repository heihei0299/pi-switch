package bootstrap

import (
	"github.com/heihei0299/pi-switch/internal/adapters/outbound/fileconfig"
	"github.com/heihei0299/pi-switch/internal/adapters/outbound/filegateway"
	"github.com/heihei0299/pi-switch/internal/config"
	domainconfig "github.com/heihei0299/pi-switch/internal/domain/config"
)

func LoadConfig(path string) (domainconfig.PiSwitchConfig, string, error) {
	return fileconfig.New(path, config.DefaultConfig).Load()
}

func GatewayModelsPath() string { return filegateway.ModelsPath() }
