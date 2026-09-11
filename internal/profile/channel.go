package profile

import "github.com/heihei0299/pi-switch/internal/config"

// EnsureMutationChannel resolves a named channel and upgrades a legacy single
// unnamed upstream to main, in place. It returns -1 when the channel does not
// exist and cannot be inferred.
func EnsureMutationChannel(prof *config.ProviderProfile, channel string) int {
	if channel == "" {
		return -1
	}
	if idx := channelIndex(*prof, channel); idx >= 0 {
		return idx
	}
	if channel != "main" || len(prof.Upstreams) > 1 {
		return -1
	}
	if len(prof.Upstreams) == 0 {
		name := "main"
		prof.Upstreams = []config.Upstream{{
			Name:          &name,
			API:           prof.API,
			ResponsesMode: prof.ResponsesMode,
			BaseURL:       prof.BaseURL,
			APIKey:        prof.APIKey,
			Headers:       prof.Headers,
		}}
		return 0
	}
	if prof.ChannelName(0) != "" {
		return -1
	}
	name := "main"
	u := &prof.Upstreams[0]
	u.Name = &name
	if u.API == "" {
		u.API = prof.API
	}
	if u.ResponsesMode == "" {
		u.ResponsesMode = prof.ResponsesMode
	}
	if u.BaseURL == "" {
		u.BaseURL = prof.BaseURL
	}
	if u.APIKey == "" {
		u.APIKey = prof.APIKey
	}
	if u.Headers == nil {
		u.Headers = prof.Headers
	}
	return 0
}

// channelIndex returns the upstream index of the named channel, or -1.
func channelIndex(prof config.ProviderProfile, channel string) int {
	for i := range prof.Upstreams {
		if prof.ChannelName(i) == channel {
			return i
		}
	}
	return -1
}
