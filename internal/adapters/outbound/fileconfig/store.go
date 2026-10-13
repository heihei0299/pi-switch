package fileconfig

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/domain/revision"
	"github.com/heihei0299/pi-switch/internal/usecase/providers"
)

var ErrBackupDirectory = errors.New("backup directory must be a private, non-symlink directory")

type Store struct{ path string }

func New(path string) *Store { return &Store{path: path} }

// shortcut: config.UpdateAtPath cannot cancel while waiting for its lock; add cancellable locking in W02.
func (s *Store) Update(ctx context.Context, backup bool, mutate func(*providers.State) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	update := func(cfg *config.PiSwitchConfig) error {
		state := stateFromConfig(*cfg)
		if err := mutate(&state); err != nil {
			return err
		}
		applyState(cfg, state)
		return nil
	}
	if backup {
		return UpdateWithBackup(s.path, update)
	}
	return config.UpdateAtPath(s.path, update)
}

func stateFromConfig(cfg config.PiSwitchConfig) providers.State {
	state := providers.State{Profiles: make(map[string]providers.StoredProfile, len(cfg.Profiles))}
	if cfg.Current != nil {
		current := *cfg.Current
		state.Current = &current
	}
	for name, prof := range cfg.Profiles {
		state.Profiles[name] = providers.StoredProfile{Profile: prof, Revision: revision.Of(prof)}
	}
	return state
}

func applyState(cfg *config.PiSwitchConfig, state providers.State) {
	profiles := make(map[string]config.ProviderProfile, len(state.Profiles))
	for name, stored := range state.Profiles {
		profiles[name] = stored.Profile
	}
	cfg.Profiles = profiles
	cfg.Current = nil
	if state.Current != nil {
		current := *state.Current
		cfg.Current = &current
	}
}

func UpdateWithBackup(path string, mutate func(*config.PiSwitchConfig) error) error {
	return config.UpdateAtPath(path, func(cfg *config.PiSwitchConfig) error {
		if err := mutate(cfg); err != nil {
			return err
		}
		_, err := CreateConfigBackup(path)
		return err
	})
}

func ValidateBackupDirectory(path string) error {
	info, err := os.Lstat(path + ".backups")
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return ErrBackupDirectory
	}
	return nil
}

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
	if err := ValidateBackupDirectory(path); err != nil {
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
