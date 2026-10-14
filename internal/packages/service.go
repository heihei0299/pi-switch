// Package packages retains the historical internal entrypoints while callers
// migrate to the typed package usecase.
package packages

import (
	"encoding/json"

	"github.com/heihei0299/pi-switch/internal/bootstrap"
	packageusecase "github.com/heihei0299/pi-switch/internal/usecase/packages"
)

var ErrPackageNotFound = packageusecase.ErrPackageNotFound

type ImportResult = packageusecase.ImportResult

const (
	StatusImported = packageusecase.StatusImported
	StatusEmpty    = packageusecase.StatusEmpty
	StatusNotFound = packageusecase.StatusNotFound
)

func PiSwitchDBPath() string { return bootstrap.PiSwitchDBPath() }

func ListInstalledPackages() ([]map[string]interface{}, error) {
	packages, err := bootstrap.NewPackagesService().ListInstalled()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(packages))
	for _, pkg := range packages {
		item, err := packageMap(pkg)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func AddInstalledPackage(spec string, enabled bool) error {
	return bootstrap.NewPackagesService().Add(packageusecase.AddCommand{Spec: spec, Enabled: enabled})
}

func GetInstalledPackage(id string) (map[string]interface{}, error) {
	pkg, err := bootstrap.NewPackagesService().GetInstalled(id)
	if err != nil {
		return nil, err
	}
	return packageMap(pkg)
}

func DeleteInstalledPackage(id string) error {
	return bootstrap.NewPackagesService().DeleteInstalled(id)
}

func ToggleInstalledPackage(id string) (bool, error) {
	return bootstrap.NewPackagesService().ToggleInstalled(id)
}

func ImportPiPackages() (ImportResult, error) {
	return bootstrap.NewPackagesService().ImportPiPackages()
}

func packageMap(pkg packageusecase.PackageDTO) (map[string]interface{}, error) {
	data, err := json.Marshal(pkg)
	if err != nil {
		return nil, err
	}
	var item map[string]interface{}
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, err
	}
	return item, nil
}
