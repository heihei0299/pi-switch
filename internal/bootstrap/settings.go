package bootstrap

import (
	"github.com/heihei0299/pi-switch/internal/adapters/outbound/fileconfig"
	"github.com/heihei0299/pi-switch/internal/config"
	settingsusecase "github.com/heihei0299/pi-switch/internal/usecase/settings"
)

func NewSettingsService(path string) settingsusecase.Service {
	return settingsusecase.NewService(fileconfig.New(path, config.DefaultConfig))
}
