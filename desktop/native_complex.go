package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

type nativeComplexPreview struct {
	tab int
	nativeJSONState
	nativeGatewayState
	nativeStatsState
	nativeSessionsState
	nativePackagesState
}

func newNativeComplexPreview() *nativeComplexPreview {
	now := time.Now()
	return &nativeComplexPreview{
		nativeStatsState:    nativeStatsState{statsFilters: newNativeStatsFilters(now)},
		nativeSessionsState: nativeSessionsState{sessionsFilters: newNativeSessionsFilters(now)},
	}
}

func (a *nativeComplexPreview) view(c *ui.Context) {
	ui.Column(c).Fill().Padding(20).Gap(12).Children(func() {
		ui.Text(c, "Native management").FontSize(24).Bold().Role(ui.RoleHeading).Level(1)
		ui.Text(c, "Validation is read-only. Publishing a draft requires explicit confirmation and writes models.json only; config and request history are not modified.").TextColor(c.Theme().TextMuted)
		ui.Segmented(c, &a.tab, "JSON Editor", "Gateway Diff", "Stats & Sessions", "Packages").Label("Prototype section")
		ui.Scroll(c).Fill().Gap(12).Children(func() {
			a.section(c, a.tab)
		})
	})
}

func (a *nativeComplexPreview) section(c *ui.Context, tab int) {
	switch tab {
	case 0:
		a.nativeJSONState.editor(c, &a.nativeGatewayState)
	case 1:
		a.nativeGatewayState.gatewayDiff(c)
	case 2:
		a.statsAndSessions(c)
	case 3:
		a.nativePackagesState.packagesSection(c)
	}
}

func (a *nativeComplexPreview) statsAndSessions(c *ui.Context) {
	if a.nativeStatsState.section(c, &a.nativeSessionsState) {
		a.nativeSessionsState.section(c, &a.nativeStatsState)
	}
}
