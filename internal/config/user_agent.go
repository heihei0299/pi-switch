package config

import domainconfig "github.com/heihei0299/pi-switch/internal/domain/config"

// ResolveUserAgentValues is shared by Proxy outbound requests and usage probes.
func ResolveUserAgentValues(profile, global *string) string {
	return domainconfig.ResolveUserAgentValues(profile, global)
}
