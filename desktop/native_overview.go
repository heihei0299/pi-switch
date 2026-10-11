package main

import (
	"github.com/heihei0299/pi-switch/internal/application"
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
	source       string
	profiles     []overviewProfile
	proxyAddress string
	webAddress   string
}

func loadNativeOverview() (nativeOverview, error) {
	snapshot, err := application.LoadManagementOverview(config.ResolvePath())
	if err != nil {
		return nativeOverview{}, err
	}
	overview := nativeOverview{source: snapshot.Source, proxyAddress: snapshot.ProxyAddress, webAddress: snapshot.WebAddress}
	for _, prof := range snapshot.Profiles {
		overview.profiles = append(overview.profiles, overviewProfile{name: prof.Name, current: prof.Current, api: prof.API, endpoint: prof.Endpoint, channels: prof.Channels, models: prof.Models, exposed: prof.Exposed})
	}
	return overview, nil
}

func setNativeCurrentProfile(name string) error {
	return application.SetCurrentProfile(config.ResolvePath(), name)
}
