package main

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"

	"github.com/heihei0299/pi-switch/internal/config"
)

type overviewProfile struct {
	name     string
	current  bool
	api      string
	endpoint string
	channels int
	models   int
	exposed  int
}

type nativeOverview struct {
	profiles     []overviewProfile
	proxyAddress string
	webAddress   string
}

func loadNativeOverview() (nativeOverview, error) {
	cfg, _, err := config.LoadConfigAtPath(config.ResolvePath())
	if err != nil {
		return nativeOverview{}, err
	}

	current := ""
	if cfg.Current != nil {
		current = *cfg.Current
	}
	overview := nativeOverview{
		proxyAddress: net.JoinHostPort(cfg.Settings.Proxy.Host, strconv.Itoa(cfg.Settings.Proxy.Port)),
		webAddress:   net.JoinHostPort(cfg.Settings.Web.Host, strconv.Itoa(cfg.Settings.Web.Port)),
	}
	for name, profile := range cfg.Profiles {
		item := overviewProfile{name: name, current: name == current, api: profile.API}
		channels := profile.ResolvedUpstreams()
		item.channels = len(channels)
		for i, channel := range channels {
			if i == 0 {
				if item.api == "" {
					item.api = channel.EffectiveAPI(profile.API)
				}
				item.endpoint = safeEndpoint(channel.BaseURL)
			}
			item.models += len(channel.Models)
			item.exposed += len(channel.ExposedModels)
		}
		overview.profiles = append(overview.profiles, item)
	}
	sort.Slice(overview.profiles, func(i, j int) bool {
		return overview.profiles[i].name < overview.profiles[j].name
	})
	return overview, nil
}

func safeEndpoint(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func setNativeCurrentProfile(name string) error {
	return config.UpdateAtPath(config.ResolvePath(), func(cfg *config.PiSwitchConfig) error {
		if _, ok := cfg.Profiles[name]; !ok {
			return fmt.Errorf("profile %q no longer exists", name)
		}
		current := name
		cfg.Current = &current
		return nil
	})
}
