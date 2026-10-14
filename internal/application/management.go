package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/heihei0299/pi-switch/internal/adapters/outbound/fileconfig"
	"github.com/heihei0299/pi-switch/internal/bootstrap"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/domain/revision"
	"github.com/heihei0299/pi-switch/internal/profile"
	providerusecase "github.com/heihei0299/pi-switch/internal/usecase/providers"
)

var ErrManagementConflict = providerusecase.ErrRevisionConflict

// Revision is a comparison token, never a second source of configuration.
func Revision(value interface{}) string { return revision.Of(value) }

// CreateConfigBackup keeps the exact pre-write bytes, including legacy fields.
func CreateConfigBackup(path string) (string, error) {
	name, err := fileconfig.CreateConfigBackup(path)
	if errors.Is(err, fileconfig.ErrBackupDirectory) {
		return "", &ManagementValidationError{Message: err.Error()}
	}
	return name, err
}

type ConfigBackup = fileconfig.Backup

func ListConfigBackups(path string) ([]ConfigBackup, error) {
	backups, err := fileconfig.ListConfigBackups(path)
	if errors.Is(err, fileconfig.ErrBackupDirectory) {
		return nil, &ManagementValidationError{Message: err.Error()}
	}
	return backups, err
}

func ConfigFileRevision(path string) (string, error) { return fileconfig.ConfigFileRevision(path) }

func RestoreConfigBackup(path, name, expected string) error {
	data, err := fileconfig.ReadConfigBackup(path, name)
	switch {
	case errors.Is(err, fileconfig.ErrInvalidBackupName):
		return &ManagementValidationError{Message: "invalid backup name"}
	case errors.Is(err, fileconfig.ErrBackupNotRegular):
		return &ManagementValidationError{Message: "backup must be a regular file"}
	case errors.Is(err, fileconfig.ErrBackupDirectory):
		return &ManagementValidationError{Message: err.Error()}
	case err != nil:
		return err
	}
	restored, err := config.ParseConfig(data)
	if err != nil {
		return &ManagementValidationError{Message: "backup is not a readable configuration"}
	}
	if err := ValidateWritableConfig(restored); err != nil {
		return &ManagementValidationError{Message: "Backup failed protocol/capability validation; nothing was restored."}
	}
	return fileconfig.New(path, config.DefaultConfig).Replace(context.Background(), func(current []byte) (config.PiSwitchConfig, error) {
		if fileconfig.ConfigRevision(current) != expected {
			return config.PiSwitchConfig{}, ErrManagementConflict
		}
		if _, err := fileconfig.CreateConfigBackup(path); err != nil {
			if errors.Is(err, fileconfig.ErrBackupDirectory) {
				return config.PiSwitchConfig{}, &ManagementValidationError{Message: err.Error()}
			}
			return config.PiSwitchConfig{}, err
		}
		return restored, nil
	})
}

// shortcut: Native provider pages retain application error mapping, upgrade when those adapters map usecase errors directly.
func SaveProvider(path, original, name string, prof config.ProviderProfile, expected string) error {
	return saveProviderContext(context.Background(), path, original, name, prof, expected)
}

func saveProviderContext(ctx context.Context, path, original, name string, prof config.ProviderProfile, expected string) error {
	_, err := bootstrap.NewProviderService(path).Save(ctx, providerusecase.SaveCommand{
		Original: original, Name: name, Profile: prof, ExpectedRevision: expected,
	})
	if errors.Is(err, providerusecase.ErrProfileExists) {
		return profile.ErrProfileExists
	}
	if errors.Is(err, providerusecase.ErrNameRequired) {
		return &ManagementValidationError{Message: err.Error()}
	}
	var validation *providerusecase.ValidationError
	if errors.As(err, &validation) {
		return providerValidationError(prof, validation.Err)
	}
	if errors.Is(err, fileconfig.ErrBackupDirectory) {
		return &ManagementValidationError{Message: err.Error()}
	}
	return err
}

func DeleteProvider(path, name, expected string) error {
	return deleteProviderContext(context.Background(), path, name, expected)
}

func deleteProviderContext(ctx context.Context, path, name, expected string) error {
	_, err := bootstrap.NewProviderService(path).Delete(ctx, providerusecase.DeleteCommand{Name: name, ExpectedRevision: expected})
	if errors.Is(err, fileconfig.ErrBackupDirectory) {
		return &ManagementValidationError{Message: err.Error()}
	}
	return err
}

func SetCurrentProfile(path, name string) error {
	err := bootstrap.NewProviderService(path).SelectCurrent(context.Background(), name)
	var validation *providerusecase.ValidationError
	if errors.As(err, &validation) {
		return errors.New(validation.Err.Error())
	}
	if errors.Is(err, fileconfig.ErrBackupDirectory) {
		return &ManagementValidationError{Message: err.Error()}
	}
	var persistence *providerusecase.PersistenceError
	if errors.As(err, &persistence) {
		return persistence.Err
	}
	return err
}

// ValidateWritableConfig is the existing full-config write gate; legacy shape
// problems remain advisory, but unsupported resolved capabilities are rejected.
func ValidateWritableConfig(cfg config.PiSwitchConfig) error {
	for name, prof := range cfg.Profiles {
		if prof.API != "" {
			if err := profile.ValidateResponsesMode(prof); err != nil {
				return fmt.Errorf("profile %s: %w", name, err)
			}
		}
		if err := config.ValidateResolvedCapability(prof); err != nil {
			return fmt.Errorf("profile %s: %w", name, err)
		}
	}
	return nil
}
