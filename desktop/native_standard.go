package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/application"
	"github.com/heihei0299/pi-switch/internal/profile"
	"github.com/heihei0299/pi-switch/internal/usecase/proxycontrol"
)

const (
	pageOverview     = "Overview"
	pageProviders    = "Providers"
	pageGateway      = "Gateway"
	pageUsage        = "Usage & Sessions"
	pageMaintenance  = "Maintenance"
	pageSettings     = "Settings"
	pageAdvancedJSON = "Advanced JSON"
	pageBackups      = "Backups"
	pagePackages     = "Packages"
	pageDoctor       = "Doctor"
)

// nativeMaintenanceGroups keeps the maintenance area grouped by purpose instead
// of piling every rare action into one ungrouped menu.
var nativeMaintenanceGroups = []struct {
	label   string
	entries []string
}{
	{"Configuration", []string{pageSettings, pageAdvancedJSON}},
	{"Recovery", []string{pageBackups}},
	{"Extensions", []string{pagePackages}},
	{"Diagnostics", []string{pageDoctor}},
}

func nativeInMaintenance(page string) bool {
	for _, group := range nativeMaintenanceGroups {
		for _, entry := range group.entries {
			if entry == page {
				return true
			}
		}
	}
	return false
}

// nativeSectionTabs maps a main page to the complex preview section it shows.
var nativeSectionTabs = map[string]int{
	pageAdvancedJSON: 0,
	pageGateway:      1,
	pageUsage:        2,
	pagePackages:     3,
}

func nativeSectionPage(page string) bool {
	_, ok := nativeSectionTabs[page]
	return ok
}

type nativeStandardPages struct {
	page      string
	providers nativeProvidersPage
	settings  nativeSettingsPage
	backups   nativeBackupsPage
	doctor    nativeDoctorPage
}

func (a *desktopShell) standardPages() *nativeStandardPages {
	if a.standard == nil {
		a.standard = &nativeStandardPages{page: pageOverview}
	}
	return a.standard
}

func (a *desktopShell) standardView(c *ui.Context) {
	pages := a.standardPages()
	ui.Column(c).Fill().Gap(8).Children(func() {
		ui.Row(c).Gap(6).Padding(6, 12).Children(func() {
			for _, page := range []string{pageOverview, pageProviders, pageGateway, pageUsage} {
				a.navigationButton(c, pages, page)
			}
			ui.Spacer(c)
			a.navigationButton(c, pages, pageMaintenance)
		})
		switch {
		case pages.page == pageMaintenance:
			a.maintenanceView(c, pages)
		case pages.page == pageOverview:
			a.view(c)
		case pages.page == pageProviders:
			ui.Scroll(c).Fill().Children(func() { pages.providers.view(a, c) })
		case pages.page == pageSettings:
			ui.Scroll(c).Fill().Children(func() { pages.settings.view(a, c) })
		case pages.page == pageBackups:
			ui.Scroll(c).Fill().Children(func() { pages.backups.view(a, c) })
		case pages.page == pageDoctor:
			ui.Scroll(c).Fill().Children(func() { pages.doctor.view(a, c) })
		case nativeSectionPage(pages.page):
			tab, _ := nativeSectionTabs[pages.page]
			ui.Scroll(c).Fill().Gap(12).Padding(20).Children(func() {
				a.complexPreview.section(c, tab)
			})
		default:
			a.view(c)
		}
	})
}

// navigationButton marks the current page without a filled accent, so a page
// still shows exactly one accent button: its own primary action.
func (a *desktopShell) navigationButton(c *ui.Context, pages *nativeStandardPages, page string) {
	current := pages.page == page
	if page == pageMaintenance {
		current = current || nativeInMaintenance(pages.page)
	}
	entry := ui.Button(c, page)
	if current {
		entry = entry.Bold().TextColor(c.Theme().Accent)
	}
	if entry.Clicked() {
		a.navigate(pages, page)
	}
}

// navigate only loads the page being opened; it never writes config, publishes
// a Gateway, or starts and stops the Proxy.
func (a *desktopShell) navigate(pages *nativeStandardPages, page string) {
	if pages.page == page {
		return
	}
	pages.page = page
	switch page {
	case pageOverview, pageProviders:
		a.refreshNativeOverview()
	case pageGateway:
		gateway := &a.complexPreview.nativeGatewayState
		if gateway.refreshGateway != nil {
			gateway.refreshGateway(nativeGatewaySelections(gateway))
		}
	case pageUsage:
		stats := &a.complexPreview.nativeStatsState
		if !stats.statsLoaded && stats.refreshStats != nil {
			stats.refreshStats(0, 0, stats.statsFilters, a.complexPreview.nativeSessionsState.sessionsFilters)
		}
	case pageSettings:
		if !pages.settings.loaded {
			pages.settings.load(a)
		}
	case pageBackups:
		pages.backups.load(a)
	case pageDoctor:
		pages.doctor.run(a)
	}
}

func (a *desktopShell) maintenanceView(c *ui.Context, pages *nativeStandardPages) {
	ui.Scroll(c).Fill().Padding(24).Gap(12).Children(func() {
		ui.Text(c, "Settings & maintenance").FontSize(26).Bold().Role(ui.RoleHeading).Level(1)
		ui.Text(c, "The four main pages cover daily work. These tools change configuration, restore backups, add packages or report problems; opening one never writes config or publishes the Gateway.").TextColor(c.Theme().TextMuted)
		for _, group := range nativeMaintenanceGroups {
			ui.Text(c, group.label).FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
			ui.Row(c).Gap(8).Children(func() {
				for _, entry := range group.entries {
					if ui.Button(c, entry).Clicked() {
						a.navigate(pages, entry)
					}
				}
			})
		}
	})
}

// Each page owns its state. This only bridges workers to the window UI thread.
func (a *desktopShell) runStandardTask(name string, run func(context.Context) (interface{}, error), apply func(interface{}, error)) bool {
	if a.taskRequested != nil {
		a.taskRequested(name)
	}
	if a.exiting || a.window == nil {
		return false
	}
	ctx, token, ok := a.tasks.begin(name)
	if !ok {
		return false
	}
	window := a.window
	window.Invalidate()
	go func() {
		result, err := run(ctx)
		if ctx.Err() != nil {
			return
		}
		window.Update(func() {
			if window.IsDestroyed() || !a.tasks.accept(name, token) {
				a.tasks.finish(name, token)
				return
			}
			a.tasks.finish(name, token)
			apply(result, err)
		})
	}()
	return true
}

func nativeManagementError(err error) string {
	if errors.Is(err, application.ErrManagementConflict) {
		return err.Error()
	}
	if errors.Is(err, proxycontrol.ErrPortInUse) {
		return "Proxy port is occupied by an unverified listener; refusing takeover. Use ss -tlnp to inspect the listener."
	}
	if errors.Is(err, profile.ErrProfileExists) {
		return "Provider name already exists; choose another name. Nothing was overwritten."
	}
	if errors.Is(err, profile.ErrProfileNotFound) {
		return "Provider no longer exists; reload before continuing."
	}
	if errors.Is(err, profile.ErrUnknownChannel) {
		return "Channel does not exist; check the channel name in the draft."
	}
	if errors.Is(err, os.ErrPermission) {
		return "Storage permission denied; check config/backup permissions. No successful write was reported."
	}
	var validation *application.ManagementValidationError
	if errors.As(err, &validation) {
		return validation.Error()
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return "Storage operation failed (" + pathErr.Op + "); no successful write was reported."
	}
	// Loader/validator messages can contain user-supplied credentials or URLs.
	return fmt.Sprintf("Operation failed (%T). Check the draft and run Doctor; credentials are not shown.", err)
}

// MyGo can enable a disabled action mid-click when blur clears preedit.
func blockComposingPress(composing bool) func(ui.InputEvent) bool {
	return func(event ui.InputEvent) bool {
		return composing && event.Kind == ui.InputPointerDown && event.Button == 0
	}
}

// blockComposingLive is blockComposingPress for a row that renders before the
// inputs it guards: it reads the composition state when the press arrives, so
// a preedit started in a lower input cannot be committed by that press.
func blockComposingLive(composing func() bool) func(ui.InputEvent) bool {
	return func(event ui.InputEvent) bool {
		return composing() && event.Kind == ui.InputPointerDown && event.Button == 0
	}
}
