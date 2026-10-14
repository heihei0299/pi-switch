package profile

import (
	"strings"

	"github.com/heihei0299/pi-switch/internal/config"
)

// DuplicateInConfig shares the capability gate with revisioned Native transactions.
func DuplicateInConfig(cfg *config.PiSwitchConfig, src, as string) error {
	as = strings.TrimSpace(as)
	if as == "" {
		return ErrTargetNameRequired
	}
	prof, ok := cfg.Profiles[src]
	if !ok {
		return profileErr(ErrProfileNotFound, "profile %q not found", src)
	}
	if _, exists := cfg.Profiles[as]; exists {
		return profileErr(ErrProfileExists, "target %q already exists", as)
	}
	// Copying remains a repair path for legacy shape/retry issues, but not for invalid API capability.
	if prof.API != "" {
		if err := ValidateResponsesMode(prof); err != nil {
			return err
		}
	}
	if err := ValidateResolvedCapability(prof); err != nil {
		return err
	}
	cfg.Profiles[as] = prof
	return nil
}
