package application

import (
	"context"

	"github.com/heihei0299/pi-switch/internal/bootstrap"
	settingsusecase "github.com/heihei0299/pi-switch/internal/usecase/settings"
)

type SettingsDraft = settingsusecase.Draft

func LoadSettingsDraft(path string) (SettingsDraft, error) {
	return bootstrap.NewSettingsService(path).LoadDraft(context.Background())
}

func SaveSettingsDraft(path, raw, expected string) (SettingsDraft, error) {
	draft, err := bootstrap.NewSettingsService(path).SaveDraft(context.Background(), raw, expected)
	return draft, mapSettingsError(err)
}
