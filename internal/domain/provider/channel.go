package provider

// EnsureMutationChannel resolves a channel and materializes the legacy flat or
// unnamed single-upstream profile as "main" when that is the requested channel.
func EnsureMutationChannel(profile *Profile, channel string) int {
	if channel == "" {
		return -1
	}
	if idx := channelIndex(*profile, channel); idx >= 0 {
		return idx
	}
	if channel != "main" || len(profile.Upstreams) > 1 {
		return -1
	}
	if len(profile.Upstreams) == 0 {
		name := "main"
		profile.Upstreams = []Upstream{{
			Name:          &name,
			API:           profile.API,
			ResponsesMode: profile.ResponsesMode,
			BaseURL:       profile.BaseURL,
			APIKey:        profile.APIKey,
			Headers:       profile.Headers,
		}}
		return 0
	}
	if profile.ChannelName(0) != "" {
		return -1
	}
	name := "main"
	upstream := &profile.Upstreams[0]
	upstream.Name = &name
	if upstream.API == "" {
		upstream.API = profile.API
	}
	if upstream.ResponsesMode == "" {
		upstream.ResponsesMode = profile.ResponsesMode
	}
	if upstream.BaseURL == "" {
		upstream.BaseURL = profile.BaseURL
	}
	if upstream.APIKey == "" {
		upstream.APIKey = profile.APIKey
	}
	if upstream.Headers == nil {
		upstream.Headers = profile.Headers
	}
	return 0
}

func channelIndex(profile Profile, channel string) int {
	for i := range profile.Upstreams {
		if profile.ChannelName(i) == channel {
			return i
		}
	}
	return -1
}
