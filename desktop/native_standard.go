package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/application"
	"github.com/heihei0299/pi-switch/internal/daemon"
	"github.com/heihei0299/pi-switch/internal/profile"
)

type nativeStandardPages struct {
	page      string
	providers nativeProvidersPage
	settings  nativeSettingsPage
	backups   nativeBackupsPage
	doctor    nativeDoctorPage
}

func (a *desktopShell) standardPages() *nativeStandardPages {
	if a.standard == nil {
		a.standard = &nativeStandardPages{page: "Overview"}
	}
	return a.standard
}

func (a *desktopShell) standardView(c *ui.Context) {
	pages := a.standardPages()
	ui.Column(c).Fill().Gap(8).Children(func() {
		for _, row := range [][]string{
			{"Overview", "Profiles", "Providers", "Proxy"},
			{"Gateway", "JSON Editor"},
			{"Stats & Sessions", "Packages"},
			{"Settings", "Backups", "Doctor"},
		} {
			ui.Row(c).Gap(6).Padding(6, 12).Children(func() {
				for _, name := range row {
					if ui.Button(c, name).Clicked() {
						pages.page = name
						switch name {
						case "Overview", "Profiles", "Providers":
							a.refreshNativeOverview()
						case "Proxy":
							a.refreshNativeOverview()
							a.proxy.refresh(a)
						case "Gateway":
							gateway := &a.complexPreview.nativeGatewayState
							if gateway.refreshGateway != nil {
								gateway.refreshGateway(nativeGatewaySelections(gateway))
							}
						case "Stats & Sessions":
							stats := &a.complexPreview.nativeStatsState
							if !stats.statsLoaded && stats.refreshStats != nil {
								stats.refreshStats(0, 0, stats.statsFilters, a.complexPreview.nativeSessionsState.sessionsFilters)
							}
						case "Settings":
							if !pages.settings.loaded {
								pages.settings.load(a)
							}
						case "Backups":
							pages.backups.load(a)
						case "Doctor":
							pages.doctor.run(a)
						}
					}
				}
			})
		}

		switch pages.page {
		case "Profiles", "Providers":
			ui.Scroll(c).Fill().Children(func() { pages.providers.view(a, c) })
		case "Proxy":
			ui.Column(c).Fill().Padding(24).Gap(12).Children(func() {
				ui.Text(c, "Proxy").FontSize(26).Bold().Role(ui.RoleHeading).Level(1)
				ui.Text(c, "Only identity-verified, managed processes can be stopped. Hiding or quitting the GUI leaves Proxy running.")
				proxyAddress, overviewError := a.overview.proxyData()
				ui.Text(c, "Configured address: "+proxyAddress)
				if overviewError != "" {
					ui.Text(c, overviewError).Role(ui.RoleStatus)
				}
				a.proxy.controls(a, c)
			})
		case "Settings":
			ui.Scroll(c).Fill().Children(func() { pages.settings.view(a, c) })
		case "Backups":
			ui.Scroll(c).Fill().Children(func() { pages.backups.view(a, c) })
		case "Doctor":
			ui.Scroll(c).Fill().Children(func() { pages.doctor.view(a, c) })
		case "Gateway", "JSON Editor", "Stats & Sessions", "Packages":
			ui.Scroll(c).Fill().Gap(12).Padding(20).Children(func() {
				switch pages.page {
				case "JSON Editor":
					a.complexPreview.section(c, 0)
				case "Gateway":
					a.complexPreview.section(c, 1)
				case "Stats & Sessions":
					a.complexPreview.section(c, 2)
				case "Packages":
					a.complexPreview.section(c, 3)
				}
			})
		default:
			a.view(c)
		}
	})
}

// Each page owns its state. This only bridges workers to the window UI thread.
func (a *desktopShell) runStandardTask(name string, run func() (interface{}, error), apply func(interface{}, error)) bool {
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
		result, err := run()
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
	if errors.Is(err, daemon.ErrPortInUse) {
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
