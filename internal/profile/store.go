package profile

import (
	"errors"

	"github.com/heihei0299/pi-switch/internal/config"
)

// loadConfig reads the shared config through the config boundary. The profile
// domain never resolves a path of its own, so HTTP and CLI read the same file.
func loadConfig() (config.PiSwitchConfig, error) {
	cfg, _, err := config.LoadConfigAtPath(config.ResolvePath())
	return cfg, err
}

type mutationError struct{ err error }

func (e mutationError) Error() string { return e.err.Error() }
func (e mutationError) Unwrap() error { return e.err }

func keepMutation(err error) error {
	if err == nil {
		return nil
	}
	return mutationError{err: err}
}

// updateConfig keeps domain validation errors intact while classifying all
// lock, load, and atomic-write failures as persistence failures.
func updateConfig(prefix string, mutate func(*config.PiSwitchConfig) error) error {
	err := config.UpdateAtPath(config.ResolvePath(), mutate)
	if err == nil {
		return nil
	}
	var domainErr mutationError
	if errors.As(err, &domainErr) {
		return domainErr.err
	}
	return profileErr(ErrPersistFailed, "%s%s", prefix, err.Error())
}
