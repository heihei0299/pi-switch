package inference

import (
	configdomain "github.com/heihei0299/pi-switch/internal/domain/config"
)

type Route struct {
	Candidates []string
	Model      string
	Channel    string
	Ambiguous  bool
}

func ResolveRoute(cfg configdomain.PiSwitchConfig, requested string) Route {
	type match struct {
		supplier string
		channel  string
	}
	var matches []match
	for name, profile := range cfg.Profiles {
		for i := range profile.Upstreams {
			for _, exposed := range profile.Upstreams[i].ExposedModels {
				if exposed == requested {
					matches = append(matches, match{name, profile.ChannelName(i)})
					break
				}
			}
		}
	}
	if len(matches) == 1 {
		return Route{Candidates: []string{matches[0].supplier}, Model: requested, Channel: matches[0].channel}
	}
	return Route{Model: requested, Ambiguous: len(matches) > 1}
}
