package application

import (
	"net"
	"net/url"
	"sort"
	"strconv"

	"github.com/heihei0299/pi-switch/internal/config"
)

type OverviewProfile struct {
	Name                      string
	Current                   bool
	API, Endpoint             string
	Channels, Models, Exposed int
}

type ManagementOverview struct {
	Source, ProxyAddress, WebAddress string
	Profiles                         []OverviewProfile
}

func LoadManagementOverview(path string) (ManagementOverview, error) {
	cfg, source, err := config.LoadConfigAtPath(path)
	if err != nil {
		return ManagementOverview{}, &ManagementValidationError{Message: "Configuration could not be read. Run Doctor and check syntax/permissions; stored values are hidden."}
	}
	current := ""
	if cfg.Current != nil {
		current = *cfg.Current
	}
	overview := ManagementOverview{Source: source, ProxyAddress: net.JoinHostPort(cfg.Settings.Proxy.Host, strconv.Itoa(cfg.Settings.Proxy.Port)), WebAddress: net.JoinHostPort(cfg.Settings.Web.Host, strconv.Itoa(cfg.Settings.Web.Port))}
	for name, prof := range cfg.Profiles {
		item := OverviewProfile{Name: name, Current: name == current, API: prof.API}
		channels := prof.ResolvedUpstreams()
		item.Channels = len(channels)
		for i, channel := range channels {
			if i == 0 {
				if item.API == "" {
					item.API = channel.EffectiveAPI(prof.API)
				}
				if parsed, err := url.Parse(channel.BaseURL); err == nil && parsed.Scheme != "" && parsed.Host != "" {
					item.Endpoint = parsed.Scheme + "://" + parsed.Host
				}
			}
			item.Models += len(channel.Models)
			item.Exposed += len(channel.ExposedModels)
		}
		overview.Profiles = append(overview.Profiles, item)
	}
	sort.Slice(overview.Profiles, func(i, j int) bool { return overview.Profiles[i].Name < overview.Profiles[j].Name })
	return overview, nil
}
