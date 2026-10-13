package fileconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	domainconfig "github.com/heihei0299/pi-switch/internal/domain/config"
	"github.com/heihei0299/pi-switch/internal/domain/revision"
	"github.com/heihei0299/pi-switch/internal/usecase/providers"
)

var ErrConfigWriteLockBusy = errors.New("config write lock busy")

const configWriteLockWait = 5 * time.Second

type Store struct {
	path     string
	defaults func() domainconfig.PiSwitchConfig
}

func New(path string, defaults func() domainconfig.PiSwitchConfig) *Store {
	return &Store{path: path, defaults: defaults}
}

func (s *Store) Load() (domainconfig.PiSwitchConfig, string, error) {
	return loadConfigAtPath(s.path, s.defaults)
}

func (s *Store) Save(cfg domainconfig.PiSwitchConfig) error {
	return withWriteLock(context.Background(), s.path, func() error {
		return saveAtPathUnlocked(cfg, s.path)
	})
}

func (s *Store) UpdateConfig(ctx context.Context, backup bool, mutate func(*domainconfig.PiSwitchConfig) error) error {
	if mutate == nil {
		return errors.New("config mutation callback is nil")
	}
	return withWriteLock(ctx, s.path, func() error {
		cfg, _, err := loadConfigAtPath(s.path, s.defaults)
		if err != nil {
			return err
		}
		if err := mutate(&cfg); err != nil {
			return err
		}
		if backup {
			if _, err := CreateConfigBackup(s.path); err != nil {
				return err
			}
		}
		return saveAtPathUnlocked(cfg, s.path)
	})
}

func (s *Store) Replace(ctx context.Context, prepare func([]byte) (domainconfig.PiSwitchConfig, error)) error {
	if prepare == nil {
		return errors.New("config replacement callback is nil")
	}
	return withWriteLock(ctx, s.path, func() error {
		current, err := os.ReadFile(s.path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		replacement, err := prepare(current)
		if err != nil {
			return err
		}
		return saveAtPathUnlocked(replacement, s.path)
	})
}

// Update implements the provider storage port with the same config transaction.
func (s *Store) Update(ctx context.Context, backup bool, mutate func(*providers.State) error) error {
	return s.UpdateConfig(ctx, backup, func(cfg *domainconfig.PiSwitchConfig) error {
		state := stateFromConfig(*cfg)
		if err := mutate(&state); err != nil {
			return err
		}
		applyState(cfg, state)
		return nil
	})
}

func loadConfigAtPath(path string, defaults func() domainconfig.PiSwitchConfig) (domainconfig.PiSwitchConfig, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if defaults == nil {
				return domainconfig.PiSwitchConfig{}, "", errors.New("config defaults are unavailable")
			}
			return defaults(), "default (no file)", nil
		}
		return domainconfig.PiSwitchConfig{}, "", fmt.Errorf("read config %s: %w", path, err)
	}
	if defaults == nil {
		return domainconfig.PiSwitchConfig{}, "", errors.New("config defaults are unavailable")
	}
	cfg, err := domainconfig.ParseConfig(data, defaults())
	if err != nil {
		return domainconfig.PiSwitchConfig{}, "", fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, path, nil
}

func saveAtPathUnlocked(cfg domainconfig.PiSwitchConfig, path string) error {
	cfg = domainconfig.MigratedForSave(cfg)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'))
}

func writeFileAtomic(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmp := file.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func withWriteLock(ctx context.Context, path string, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(path+".lock", os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	deadline := time.Now().Add(configWriteLockWait)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		locked, err := tryConfigWriteLock(file)
		if err != nil {
			return err
		}
		if locked {
			defer unlockConfigWriteLock(file)
			if err := ctx.Err(); err != nil {
				return err
			}
			return fn()
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: %s", ErrConfigWriteLockBusy, path)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func stateFromConfig(cfg domainconfig.PiSwitchConfig) providers.State {
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

func applyState(cfg *domainconfig.PiSwitchConfig, state providers.State) {
	profiles := make(map[string]domainconfig.ProviderProfile, len(state.Profiles))
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
