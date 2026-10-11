package application

import (
	"encoding/json"

	"github.com/heihei0299/pi-switch/internal/config"
)

type SettingsDraft struct {
	JSON     string
	Revision string
}

func settingsDraft(settings config.Settings) (SettingsDraft, error) {
	data, err := json.MarshalIndent(settings, "", "  ")
	return SettingsDraft{JSON: string(data), Revision: Revision(settings)}, err
}

func LoadSettingsDraft(path string) (SettingsDraft, error) {
	cfg, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		return SettingsDraft{}, err
	}
	return settingsDraft(cfg.Settings)
}

func SaveSettingsDraft(path, raw, expected string) (SettingsDraft, error) {
	var settings config.Settings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return SettingsDraft{}, &ManagementValidationError{Message: "invalid settings JSON; draft retained"}
	}
	if err := SaveSettings(path, settings, expected); err != nil {
		return SettingsDraft{}, err
	}
	return settingsDraft(config.MigratedForSave(config.PiSwitchConfig{Settings: settings}).Settings)
}
