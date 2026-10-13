package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

func updateWithBackup(path string, mutate func(*config.PiSwitchConfig) error) error {
	err := fileconfig.UpdateWithBackup(path, mutate)
	if errors.Is(err, fileconfig.ErrBackupDirectory) {
		return &ManagementValidationError{Message: err.Error()}
	}
	return err
}

func validateBackupDirectory(path string) error {
	err := fileconfig.ValidateBackupDirectory(path)
	if errors.Is(err, fileconfig.ErrBackupDirectory) {
		return &ManagementValidationError{Message: err.Error()}
	}
	return err
}

// CreateConfigBackup keeps the exact pre-write bytes, including legacy fields.
func CreateConfigBackup(path string) (string, error) {
	name, err := fileconfig.CreateConfigBackup(path)
	if errors.Is(err, fileconfig.ErrBackupDirectory) {
		return "", &ManagementValidationError{Message: err.Error()}
	}
	return name, err
}

type ConfigBackup struct {
	Name       string
	Size       int64
	ModifiedAt time.Time
}

func ListConfigBackups(path string) ([]ConfigBackup, error) {
	if err := validateBackupDirectory(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []ConfigBackup{}, nil
		}
		return nil, err
	}
	entries, err := os.ReadDir(path + ".backups")
	if errors.Is(err, os.ErrNotExist) {
		return []ConfigBackup{}, nil
	}
	if err != nil {
		return nil, err
	}
	backups := make([]ConfigBackup, 0)
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), "config-") && strings.HasSuffix(entry.Name(), ".json") {
			info, err := entry.Info()
			if err != nil {
				return nil, err
			}
			backups = append(backups, ConfigBackup{Name: entry.Name(), Size: info.Size(), ModifiedAt: info.ModTime()})
		}
	}
	sort.Slice(backups, func(i, j int) bool {
		if !backups[i].ModifiedAt.Equal(backups[j].ModifiedAt) {
			return backups[i].ModifiedAt.After(backups[j].ModifiedAt)
		}
		return backups[i].Name < backups[j].Name
	})
	return backups, nil
}

func configFileRevision(data []byte) string {
	if data == nil {
		return "missing-config"
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ConfigFileRevision(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return configFileRevision(data), nil
}

func RestoreConfigBackup(path, name, expected string) error {
	if name != filepath.Base(name) || !strings.HasPrefix(name, "config-") || !strings.HasSuffix(name, ".json") {
		return &ManagementValidationError{Message: "invalid backup name"}
	}
	if err := validateBackupDirectory(path); err != nil {
		return err
	}
	backupPath := filepath.Join(path+".backups", name)
	info, err := os.Lstat(backupPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return &ManagementValidationError{Message: "backup must be a regular file"}
	}
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}
	restored, err := config.ParseConfig(data)
	if err != nil {
		return &ManagementValidationError{Message: "backup is not a readable configuration"}
	}
	if err := ValidateWritableConfig(restored); err != nil {
		return &ManagementValidationError{Message: "Backup failed protocol/capability validation; nothing was restored."}
	}
	return config.ReplaceAtPath(path, func(current []byte) (config.PiSwitchConfig, error) {
		if configFileRevision(current) != expected {
			return config.PiSwitchConfig{}, ErrManagementConflict
		}
		if _, err := CreateConfigBackup(path); err != nil {
			return config.PiSwitchConfig{}, err
		}
		return restored, nil
	})
}

func SaveSettings(path string, settings config.Settings, expected string) error {
	if err := config.ValidateSettingsRetry(settings); err != nil {
		return &ManagementValidationError{Message: "Invalid proxy retry settings; check retry counts, intervals and requestScopedErrors status/match/action rules."}
	}
	if settings.Proxy.Port < 1 || settings.Proxy.Port > 65535 || settings.Web.Port < 1 || settings.Web.Port > 65535 {
		return &ManagementValidationError{Message: "ports must be between 1 and 65535"}
	}
	return updateWithBackup(path, func(cfg *config.PiSwitchConfig) error {
		if Revision(cfg.Settings) != expected {
			return ErrManagementConflict
		}
		cfg.Settings = settings
		return nil
	})
}

// shortcut: keep caller-compatible Provider facades until entrypoint wiring is folded in W08.
// PutLegacyProvider preserves the historical HTTP overwrite and rename semantics.
func PutLegacyProvider(ctx context.Context, path, name, renameFrom string, prof config.ProviderProfile) error {
	_, err := bootstrap.NewProviderService(path).PutLegacy(ctx, providerusecase.PutLegacyCommand{
		Name: name, RenameFrom: renameFrom, Profile: prof,
	})
	return err
}

func DeleteLegacyProvider(ctx context.Context, path, name string) error {
	_, err := bootstrap.NewProviderService(path).DeleteLegacy(ctx, providerusecase.DeleteLegacyCommand{Name: name})
	return err
}

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
	return updateWithBackup(path, func(cfg *config.PiSwitchConfig) error {
		if _, ok := cfg.Profiles[name]; !ok {
			return fmt.Errorf("profile %q no longer exists", name)
		}
		cfg.Current = &name
		return nil
	})
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
