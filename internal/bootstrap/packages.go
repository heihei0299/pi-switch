package bootstrap

import (
	"github.com/heihei0299/pi-switch/internal/adapters/outbound/piagentpackages"
	"github.com/heihei0299/pi-switch/internal/adapters/outbound/sqlpackages"
	packageusecase "github.com/heihei0299/pi-switch/internal/usecase/packages"
)

func PiSwitchDBPath() string { return sqlpackages.PiSwitchDBPath() }

func NewPackagesService() packageusecase.Service {
	return packageusecase.NewService(sqlpackages.New(""), piagentpackages.Source{})
}
