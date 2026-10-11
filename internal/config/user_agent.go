package config

// ResolveUserAgentValues is shared by Proxy outbound requests and usage probes.
func ResolveUserAgentValues(profile, global *string) string {
	if profile != nil && *profile != "" {
		return *profile
	}
	if global != nil && *global != "" {
		return *global
	}
	return "curl/8.5.0"
}
