package profile

import (
	"github.com/heihei0299/pi-switch/internal/config"
	domainprovider "github.com/heihei0299/pi-switch/internal/domain/provider"
)

// EnsureMutationChannel preserves the legacy profile API while the rule lives in domain.
func EnsureMutationChannel(prof *config.ProviderProfile, channel string) int {
	return domainprovider.EnsureMutationChannel(prof, channel)
}
