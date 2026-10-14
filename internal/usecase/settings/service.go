package settings

import (
	"context"
	"encoding/json"
	"errors"

	domainconfig "github.com/heihei0299/pi-switch/internal/domain/config"
	"github.com/heihei0299/pi-switch/internal/domain/revision"
)

var ErrRevisionConflict = errors.New("configuration changed; reload before saving")

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

type Draft struct {
	JSON     string
	Revision string
}

type ConfigStore interface {
	Load() (domainconfig.PiSwitchConfig, string, error)
	UpdateConfig(context.Context, bool, func(*domainconfig.PiSwitchConfig) error) error
}

type Service struct{ store ConfigStore }

func NewService(store ConfigStore) Service { return Service{store: store} }

func (s Service) Read(context.Context) (domainconfig.Settings, error) {
	cfg, _, err := s.store.Load()
	if err != nil {
		return domainconfig.Settings{}, err
	}
	return cfg.Settings, nil
}

func (s Service) LoadDraft(ctx context.Context) (Draft, error) {
	settings, err := s.Read(ctx)
	if err != nil {
		return Draft{}, err
	}
	return settingsDraft(settings)
}

// PutLegacy preserves HTTP PUT /api/settings: latest-file update, no revision
// precondition, no pre-write backup, and retry validation only.
func (s Service) PutLegacy(ctx context.Context, settings domainconfig.Settings) error {
	if err := domainconfig.ValidateSettingsRetry(settings); err != nil {
		return &ValidationError{Message: err.Error()}
	}
	return s.store.UpdateConfig(ctx, false, func(cfg *domainconfig.PiSwitchConfig) error {
		cfg.Settings = settings
		return nil
	})
}

// Save preserves the Native settings contract: validate, compare current
// revision under the file lock, create a pre-write backup, then save atomically.
func (s Service) Save(ctx context.Context, settings domainconfig.Settings, expected string) error {
	if err := domainconfig.ValidateSettingsRetry(settings); err != nil {
		return &ValidationError{Message: "Invalid proxy retry settings; check retry counts, intervals and requestScopedErrors status/match/action rules."}
	}
	if settings.Proxy.Port < 1 || settings.Proxy.Port > 65535 || settings.Web.Port < 1 || settings.Web.Port > 65535 {
		return &ValidationError{Message: "ports must be between 1 and 65535"}
	}
	return s.store.UpdateConfig(ctx, true, func(cfg *domainconfig.PiSwitchConfig) error {
		if revision.Of(cfg.Settings) != expected {
			return ErrRevisionConflict
		}
		cfg.Settings = settings
		return nil
	})
}

func (s Service) SaveDraft(ctx context.Context, raw, expected string) (Draft, error) {
	var settings domainconfig.Settings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return Draft{}, &ValidationError{Message: "invalid settings JSON; draft retained"}
	}
	if err := s.Save(ctx, settings, expected); err != nil {
		return Draft{}, err
	}
	return settingsDraft(domainconfig.MigratedForSave(domainconfig.PiSwitchConfig{Settings: settings}).Settings)
}

func settingsDraft(settings domainconfig.Settings) (Draft, error) {
	data, err := json.MarshalIndent(settings, "", "  ")
	return Draft{JSON: string(data), Revision: revision.Of(settings)}, err
}
