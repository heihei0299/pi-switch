package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/profile"
)

var ErrManagementConflict = errors.New("configuration changed; reload before saving")

// Revision is a comparison token, never a second source of configuration.
func Revision(value interface{}) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func updateWithBackup(path string, mutate func(*config.PiSwitchConfig) error) error {
	return config.UpdateAtPath(path, func(cfg *config.PiSwitchConfig) error {
		if err := mutate(cfg); err != nil {
			return err
		}
		_, err := CreateConfigBackup(path)
		return err
	})
}

func validateBackupDirectory(path string) error {
	info, err := os.Lstat(path + ".backups")
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return &ManagementValidationError{Message: "backup directory must be a private, non-symlink directory"}
	}
	return nil
}

// CreateConfigBackup keeps the exact pre-write bytes, including legacy fields.
func CreateConfigBackup(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(path+".backups", 0700); err != nil {
		return "", err
	}
	if err := validateBackupDirectory(path); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(path+".backups", "config-*.json")
	if err != nil {
		return "", err
	}
	complete := false
	defer func() {
		_ = file.Close()
		if !complete {
			_ = os.Remove(file.Name())
		}
	}()
	if err := file.Chmod(0600); err != nil {
		return "", err
	}
	if _, err := file.Write(data); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	complete = true
	return filepath.Base(file.Name()), nil
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

func SaveProvider(path, original, name string, prof config.ProviderProfile, expected string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return &ManagementValidationError{Message: "name required"}
	}
	if err := profile.ValidateProfile(prof); err != nil {
		return providerValidationError(prof, err)
	}
	return updateWithBackup(path, func(cfg *config.PiSwitchConfig) error {
		if cfg.Profiles == nil {
			cfg.Profiles = make(map[string]config.ProviderProfile)
		}
		if original != "" {
			old, ok := cfg.Profiles[original]
			if !ok || Revision(old) != expected {
				return ErrManagementConflict
			}
		}
		if _, exists := cfg.Profiles[name]; exists && name != original {
			return profile.ErrProfileExists
		}
		if original != name {
			delete(cfg.Profiles, original)
			if cfg.Current != nil && *cfg.Current == original {
				cfg.Current = &name
			}
		}
		cfg.Profiles[name] = prof
		return nil
	})
}

func DeleteProvider(path, name, expected string) error {
	return updateWithBackup(path, func(cfg *config.PiSwitchConfig) error {
		prof, ok := cfg.Profiles[name]
		if !ok || Revision(prof) != expected {
			return ErrManagementConflict
		}
		delete(cfg.Profiles, name)
		if cfg.Current != nil && *cfg.Current == name {
			cfg.Current = nil
		}
		return nil
	})
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
