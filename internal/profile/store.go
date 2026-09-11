package profile

import "github.com/heihei0299/pi-switch/internal/config"

// loadConfig reads the shared config through the config boundary. The profile
// domain never resolves a path of its own, so HTTP and CLI read the same file.
func loadConfig() (config.PiSwitchConfig, error) {
	cfg, _, err := config.LoadConfigAtPath(config.ResolvePath())
	return cfg, err
}

// persist writes the config and tags a failure with ErrPersistFailed. prefix is
// what the caller used to print in front of the raw error ("failed to save
// config: " for most endpoints, "" for the one that answered raw), so the kind is
// added without rewriting any 500 body.
func persist(cfg config.PiSwitchConfig, prefix string) error {
	if err := config.SaveAtPath(cfg, config.ResolvePath()); err != nil {
		return profileErr(ErrPersistFailed, "%s%s", prefix, err.Error())
	}
	return nil
}
