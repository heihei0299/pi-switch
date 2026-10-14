package config

// ResolveUserAgentValues chooses the most specific configured user agent.
func ResolveUserAgentValues(profile, global *string) string {
	if profile != nil && *profile != "" {
		return *profile
	}
	if global != nil && *global != "" {
		return *global
	}
	return "curl/8.5.0"
}
