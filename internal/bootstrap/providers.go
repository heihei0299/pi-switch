package bootstrap

import (
	"github.com/heihei0299/pi-switch/internal/adapters/outbound/fileconfig"
	"github.com/heihei0299/pi-switch/internal/adapters/outbound/profilevalidation"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/usecase/providers"
)

func NewProviderService(path string) *providers.Service {
	return providers.New(fileconfig.New(path, config.DefaultConfig), profilevalidation.Validator{})
}
