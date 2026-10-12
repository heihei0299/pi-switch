package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/daemon"
	"github.com/heihei0299/pi-switch/internal/gateway"
	statsservice "github.com/heihei0299/pi-switch/internal/stats"
	"github.com/heihei0299/pi-switch/internal/store"
)

type desktopShell struct {
	window                 *mygo.Window
	tray                   *mygo.Tray
	startMinimized         bool
	exiting                bool
	busy                   bool
	proxyMessage           string
	trayMessage            string
	overview               nativeOverview
	standard               *nativeStandardPages
	proxyStatus            nativeProxyStatus
	complexPreview         *nativeComplexPreview
	overviewError          string
	overviewBusy           bool
	overviewRefreshPending bool
	profileQuery           string
	profileList            ui.ListState
	selectedProfile        int
	profileSwitching       bool
	profileSwitchMessage   string
	tasks                  desktopTasks
}

func startsMinimized(args []string) bool {
	for _, arg := range args {
		if arg == "--start-minimized" {
			return true
		}
	}
	return false
}

// ponytail: CLI discovery uses resources, PATH or PI_SWITCH_CLI_PATH until WP-07 bundles target binaries.
func proxyExecutable() (string, error) {
	name := "pi-switch"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if path := os.Getenv("PI_SWITCH_CLI_PATH"); path != "" {
		return executablePath(path)
	}
	if resources, err := mygo.App.Path(mygo.PathResources); err == nil {
		if path, err := executablePath(filepath.Join(resources, name)); err == nil {
			return path, nil
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return filepath.Abs(path)
	}
	return "", fmt.Errorf("pi-switch CLI not found; bundle it beside the desktop app or set PI_SWITCH_CLI_PATH")
}

func executablePath(path string) (string, error) {
	found, err := exec.LookPath(path)
	if err != nil {
		return "", fmt.Errorf("pi-switch CLI %q is unavailable: %w", path, err)
	}
	return filepath.Abs(found)
}

func (a *desktopShell) proxyService() (daemon.Service, error) {
	executable, err := proxyExecutable()
	if err != nil {
		return daemon.Service{}, err
	}
	service := daemon.Proxy
	service.Executable = executable
	return service, nil
}

func desktopProxyAddress() (string, uint16, error) {
	cfg, _, err := config.LoadConfigAtPath(config.ResolvePath())
	if err != nil {
		return "", 0, err
	}
	port := cfg.Settings.Proxy.Port
	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("proxy port %d is outside the valid range", port)
	}
	return cfg.Settings.Proxy.Host, uint16(port), nil
}

func (a *desktopShell) startProxy() {
	a.runProxyTask(func() (daemon.DaemonResult, error) {
		status, err := daemon.Status(daemon.Proxy)
		if err != nil || status.Running {
			return status, err
		}
		host, port, err := desktopProxyAddress()
		if err != nil {
			return daemon.DaemonResult{}, err
		}
		service, err := a.proxyService()
		if err != nil {
			return daemon.DaemonResult{}, err
		}
		return daemon.Start(service, host, port)
	})
}

func (a *desktopShell) stopProxy() {
	a.runProxyTask(func() (daemon.DaemonResult, error) {
		return daemon.Stop(daemon.Proxy)
	})
}

func (a *desktopShell) refreshProxy() {
	a.runProxyTask(func() (daemon.DaemonResult, error) {
		return daemon.Status(daemon.Proxy)
	})
}

func (a *desktopShell) runProxyTask(run func() (daemon.DaemonResult, error)) {
	if a.busy || a.exiting {
		return
	}
	window := a.window
	if window == nil || window.IsDestroyed() {
		return
	}
	ctx, token, ok := a.tasks.begin("proxy")
	if !ok {
		return
	}
	a.busy = true
	window.Invalidate()
	go func() {
		result, err := run()
		if ctx.Err() != nil {
			return
		}
		message := result.Message
		if err != nil {
			message = nativeManagementError(err)
		}
		checkedAt := time.Now().Format(time.RFC3339)
		window.Update(func() {
			if window.IsDestroyed() || !a.tasks.accept("proxy", token) {
				a.tasks.finish("proxy", token)
				return
			}
			defer a.tasks.finish("proxy", token)
			a.busy = false
			a.proxyMessage = message
			a.proxyStatus = nativeProxyStatus{result: result, checked: true, failed: err != nil, checkedAt: checkedAt}
			if a.window != nil {
				a.window.Invalidate()
			}
		})
	}()
}

func (a *desktopShell) showWindow() {
	if a.window == nil {
		return
	}
	a.window.Restore()
	a.window.Show()
	a.window.Focus()
}

func (a *desktopShell) applicationMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "File", Submenu: []*mygo.MenuItem{
			{Label: "Show pi-switch", Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
			{Label: "Refresh Proxy Status", Click: func(*mygo.MenuItem, *mygo.Window) { a.refreshProxy() }},
			{Label: "Start Proxy", Click: func(*mygo.MenuItem, *mygo.Window) { a.startProxy() }},
			{Label: "Stop Proxy", Click: func(*mygo.MenuItem, *mygo.Window) { a.stopProxy() }},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Role: mygo.RoleEditMenu},
	})
}

func (a *desktopShell) trayMenu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Label: "Show pi-switch", Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
		{Label: "Refresh Proxy Status", Click: func(*mygo.MenuItem, *mygo.Window) { a.refreshProxy() }},
		{Label: "Start Proxy", Click: func(*mygo.MenuItem, *mygo.Window) { a.startProxy() }},
		{Label: "Stop Proxy", Click: func(*mygo.MenuItem, *mygo.Window) { a.stopProxy() }},
		mygo.Separator(),
		{Label: "Quit", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
	})
}

func (a *desktopShell) createTray() {
	resources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		a.setTrayError(err)
		return
	}
	icon, err := os.ReadFile(filepath.Join(resources, "icon.png"))
	if err != nil {
		a.setTrayError(err)
		return
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:    icon,
		ToolTip: "pi-switch",
		Menu:    a.trayMenu(),
	})
	if err != nil {
		a.setTrayError(err)
		return
	}
	a.tray = tray
}

func (a *desktopShell) setTrayError(err error) {
	a.trayMessage = "System tray unavailable; use File > Show pi-switch (Alt+F10 on Linux). " + err.Error()
	log.Printf("desktop tray unavailable: %v", err)
}

func (a *desktopShell) startsHidden() bool {
	return a.startMinimized && a.tray != nil
}

func (a *desktopShell) primaryWindowOptions() mygo.WindowOptions {
	return a.windowOptions(a.startsHidden())
}

func (a *desktopShell) windowOptions(hidden bool) mygo.WindowOptions {
	return mygo.WindowOptions{Title: "pi-switch — Native Management", Width: 1000, Height: 760, MinWidth: 560, MinHeight: 420, BackgroundColor: "light-dark(#f6f7f9, #0f1115)", StateKey: "native-overview", Hidden: hidden, AutoHideMenuBar: true, Content: ui.View(a.standardView)}
}

func (a *desktopShell) start() {
	a.createTray()
	a.window = mygo.NewWindow(a.primaryWindowOptions())
	a.window.OnClose(func(e *mygo.CloseEvent) {
		if a.onWindowClose(e) {
			mygo.App.Quit()
		}
	})
	a.refreshNativeOverview()
	a.refreshProxy()
}

func (a *desktopShell) runNativePackageTask(task func() (string, error)) {
	if a.complexPreview == nil || a.window == nil || a.complexPreview.packagesLoading || a.exiting {
		return
	}
	preview := a.complexPreview
	window := a.window
	ctx, token, ok := a.tasks.begin("packages")
	if !ok {
		return
	}
	preview.packagesLoading = true
	preview.packagesError = ""
	window.Invalidate()
	go func() {
		message, err := task()
		if ctx.Err() != nil {
			return
		}
		var packages []nativePackage
		var listErr error
		if err == nil {
			packages, listErr = loadNativePackages()
		}
		window.Update(func() {
			if window.IsDestroyed() || !a.tasks.accept("packages", token) {
				a.tasks.finish("packages", token)
				return
			}
			defer a.tasks.finish("packages", token)
			preview.packagesLoading = false
			preview.packagesLoaded = true
			if err != nil {
				preview.packagesError = err.Error()
				return
			}
			if listErr != nil {
				preview.packagesError = listErr.Error()
				return
			}
			preview.packages = packages
			preview.packagesError = ""
			preview.packagesMessage = message
		})
	}()
}

func (a *desktopShell) refreshNativeGatewayPreview(selections []gateway.GatewaySelection) {
	if a.complexPreview == nil || a.window == nil || a.complexPreview.gatewayLoading || a.complexPreview.gatewayPublishing || a.exiting {
		return
	}
	preview := a.complexPreview
	window := a.window
	ctx, token, ok := a.tasks.begin("gateway")
	if !ok {
		return
	}
	preview.gatewayLoading = true
	preview.gatewayError = ""
	window.Invalidate()
	go func() {
		loaded, err := loadNativeGatewayPreviewForSelection(selections)
		if ctx.Err() != nil {
			return
		}
		window.Update(func() {
			if window.IsDestroyed() || !a.tasks.accept("gateway", token) {
				a.tasks.finish("gateway", token)
				return
			}
			defer a.tasks.finish("gateway", token)
			preview.gatewayLoading = false
			if err != nil {
				preview.gatewayError = err.Error()
				return
			}
			preview.gateway = loaded
			preview.gatewayLoaded = true
			preview.gatewaySelectionExplicit = selections != nil
			preview.gatewayError = ""
		})
	}()
}

func (a *desktopShell) publishNativeGatewaySelection(selections []gateway.GatewaySelection) {
	if a.complexPreview == nil || a.window == nil || a.complexPreview.gatewayLoading || a.complexPreview.gatewayPublishing || a.exiting {
		return
	}
	preview := a.complexPreview
	window := a.window
	ctx, token, ok := a.tasks.begin("gateway")
	if !ok {
		return
	}
	preview.gatewayPublishing = true
	preview.gatewayPublishMessage = ""
	window.Invalidate()
	go func() {
		loaded, err := publishNativeGatewaySelection(selections)
		if ctx.Err() != nil {
			return
		}
		window.Update(func() {
			if window.IsDestroyed() || !a.tasks.accept("gateway", token) {
				a.tasks.finish("gateway", token)
				return
			}
			defer a.tasks.finish("gateway", token)
			preview.gatewayPublishing = false
			if err != nil {
				preview.gatewayPublishMessage = "Publish failed: " + err.Error()
				return
			}
			preview.gateway = loaded
			preview.gatewayLoaded = true
			preview.gatewaySelectionExplicit = selections != nil
			preview.gatewayPublishMessage = "Published selected Gateway models."
		})
	}()
}

func (a *desktopShell) publishNativeGatewayDraft(raw string) {
	if a.complexPreview == nil || a.window == nil || a.complexPreview.gatewayLoading || a.complexPreview.gatewayPublishing || a.exiting {
		return
	}
	preview := a.complexPreview
	window := a.window
	ctx, token, ok := a.tasks.begin("gateway")
	if !ok {
		return
	}
	preview.gatewayPublishing = true
	preview.gatewayPublishMessage = ""
	preview.jsonStatus = "Publishing Gateway draft…"
	window.Invalidate()
	go func() {
		loaded, err := publishNativeGatewayDraft(raw)
		if ctx.Err() != nil {
			return
		}
		window.Update(func() {
			if window.IsDestroyed() || !a.tasks.accept("gateway", token) {
				a.tasks.finish("gateway", token)
				return
			}
			defer a.tasks.finish("gateway", token)
			preview.gatewayPublishing = false
			if err != nil {
				preview.gatewayPublishMessage = "Publish failed: " + err.Error()
				preview.jsonStatus = preview.gatewayPublishMessage
				return
			}
			preview.gateway = loaded
			preview.gatewayLoaded = true
			preview.gatewaySelectionExplicit = false
			preview.gatewayPublishMessage = "Published Gateway draft."
			preview.jsonStatus = preview.gatewayPublishMessage
			preview.invalidateJSONDraftValidation()
		})
	}()
}

func (a *desktopShell) refreshNativeStats(requestPage, conversationPage int, filters nativeStatsFilters) {
	if a.complexPreview == nil || a.window == nil || a.complexPreview.statsLoading || a.exiting {
		return
	}
	preview := a.complexPreview
	window := a.window
	ctx, token, ok := a.tasks.begin("stats")
	if !ok {
		return
	}
	preview.statsLoading = true
	preview.statsError = ""
	window.Invalidate()
	go func() {
		loaded, err := loadNativeStats(requestPage, conversationPage, 10, filters)
		if ctx.Err() != nil {
			return
		}
		window.Update(func() {
			if window.IsDestroyed() || !a.tasks.accept("stats", token) {
				a.tasks.finish("stats", token)
				return
			}
			nextRequestPage, nextConversationPage, filtersChanged := nativeStatsPagesForLatestFilters(requestPage, conversationPage, filters, preview.statsFilters)
			if filtersChanged {
				preview.statsLoading = false
				a.tasks.finish("stats", token)
				a.refreshNativeStats(nextRequestPage, nextConversationPage, preview.statsFilters)
				return
			}
			defer a.tasks.finish("stats", token)
			preview.statsLoading = false
			if err != nil {
				preview.statsError = err.Error()
				return
			}
			preview.stats = loaded
			preview.statsLoaded = true
			preview.statsError = ""
			preview.selectedUsage = -1
		})
	}()
}

func (a *desktopShell) exportNativeStats(format string) {
	if a.complexPreview == nil || a.window == nil || a.complexPreview.statsExporting || a.exiting || (format != "json" && format != "csv") {
		return
	}
	preview := a.complexPreview
	window := a.window
	ctx, token, ok := a.tasks.begin("stats-export")
	if !ok {
		return
	}
	preview.statsExporting = true
	preview.statsExportStatus = "Preparing request history export…"
	window.Invalidate()
	go func() {
		var status string
		if _, err := os.Stat(store.DBPath()); errors.Is(err, os.ErrNotExist) {
			status = "No request history to export."
		} else if err != nil {
			status = "Export failed: inspect request history: " + err.Error()
		} else {
			db, err := store.GetDB()
			if err != nil {
				status = "Export failed: " + err.Error()
			} else {
				service := statsservice.Service{DB: db}
				filter := mygo.FileFilter{Name: "JSON", Extensions: []string{"json"}}
				if format == "csv" {
					filter = mygo.FileFilter{Name: "CSV", Extensions: []string{"csv"}}
				}
				path, saveErr := mygo.Dialog.Save(mygo.SaveDialogOptions{
					Parent: window, Title: "Export request history", DefaultPath: "pi-switch-logs." + format,
					Filters: []mygo.FileFilter{filter},
				})
				if saveErr != nil {
					status = "Export failed: " + saveErr.Error()
				} else if path == "" {
					status = "Export canceled."
				} else if ctx.Err() == nil {
					if err := writeNativeStatsExport(path, format, service); err != nil {
						status = "Export failed: " + err.Error()
					} else {
						status = "Exported request history to " + filepath.Base(path)
					}
				}
			}
		}
		if ctx.Err() != nil {
			return
		}
		window.Update(func() {
			if window.IsDestroyed() || !a.tasks.accept("stats-export", token) {
				a.tasks.finish("stats-export", token)
				return
			}
			defer a.tasks.finish("stats-export", token)
			preview.statsExporting = false
			preview.statsExportStatus = status
		})
	}()
}

func (a *desktopShell) refreshNativeConversationRequests(id string, page int) {
	if a.complexPreview == nil || a.window == nil || a.complexPreview.conversationRequestsLoading || a.exiting {
		return
	}
	preview := a.complexPreview
	window := a.window
	ctx, token, ok := a.tasks.begin("conversation")
	if !ok {
		return
	}
	preview.conversationRequests = nativeConversationPage{id: id, page: page}
	preview.conversationRequestsLoading = true
	preview.conversationRequestsError = ""
	window.Invalidate()
	go func() {
		loaded, err := loadNativeConversationPage(id, page, 10)
		if ctx.Err() != nil {
			return
		}
		window.Update(func() {
			if window.IsDestroyed() || !a.tasks.accept("conversation", token) {
				a.tasks.finish("conversation", token)
				return
			}
			defer a.tasks.finish("conversation", token)
			preview.conversationRequestsLoading = false
			if err != nil {
				preview.conversationRequestsError = err.Error()
				return
			}
			preview.conversationRequests = loaded
			preview.conversationRequestsError = ""
		})
	}()
}

func (a *desktopShell) refreshNativeOverview() {
	if a.window == nil || a.exiting {
		return
	}
	if a.overviewBusy || a.profileSwitching {
		a.overviewRefreshPending = true
		return
	}
	a.overviewBusy = true
	ctx, token, ok := a.tasks.begin("overview")
	if !ok {
		a.overviewBusy = false
		return
	}
	a.window.Invalidate()
	window := a.window
	go func() {
		overview, err := loadNativeOverview()
		if ctx.Err() != nil {
			return
		}
		window.Update(func() {
			if window.IsDestroyed() || !a.tasks.accept("overview", token) {
				a.tasks.finish("overview", token)
				return
			}
			a.tasks.finish("overview", token)
			a.updateNativeOverview(overview, err)
			a.flushOverviewRefresh()
		})
	}()
}

func (a *desktopShell) flushOverviewRefresh() {
	if !a.overviewRefreshPending {
		return
	}
	a.overviewRefreshPending = false
	a.refreshNativeOverview()
}

func (a *desktopShell) updateNativeOverview(overview nativeOverview, err error) {
	a.overviewBusy = false
	if err != nil {
		a.overviewError = err.Error()
		return
	}
	a.overview = overview
	a.overviewError = ""
	a.selectedProfile = -1
}

func (a *desktopShell) switchProfile(name string) {
	if a.profileSwitching || a.exiting || a.window == nil {
		return
	}
	a.profileSwitching = true
	ctx, token, ok := a.tasks.begin("overview")
	if !ok {
		a.profileSwitching = false
		return
	}
	a.profileSwitchMessage = "Switching profile…"
	window := a.window
	window.Invalidate()
	go func() {
		err := setNativeCurrentProfile(name)
		if ctx.Err() != nil {
			return
		}
		var overview nativeOverview
		var overviewErr error
		if err == nil {
			overview, overviewErr = loadNativeOverview()
		}
		window.Update(func() {
			if window.IsDestroyed() || !a.tasks.accept("overview", token) {
				a.tasks.finish("overview", token)
				return
			}
			a.tasks.finish("overview", token)
			a.profileSwitching = false
			defer a.flushOverviewRefresh()
			if err != nil {
				a.profileSwitchMessage = "Switch failed: " + nativeManagementError(err)
				return
			}
			a.profileSwitchMessage = "Switched to " + name
			if overviewErr != nil {
				a.overviewError = overviewErr.Error()
				return
			}
			a.overview = overview
			a.overviewError = ""
		})
	}()
}

func (a *desktopShell) onWindowClose(e *mygo.CloseEvent) bool {
	if a.exiting {
		return false
	}
	if a.tray == nil {
		return true
	}
	e.PreventDefault()
	if a.window != nil {
		a.window.Hide()
	}
	return false
}

func (a *desktopShell) view(c *ui.Context) {
	theme := c.Theme()
	ui.Column(c).Fill().Padding(24).Gap(12).Children(func() {
		ui.Text(c, "pi-switch").FontSize(26).Bold().Role(ui.RoleHeading).Level(1)
		ui.Text(c, "Native management").TextColor(theme.TextMuted)
		if a.overview.source != "" {
			ui.Text(c, "Config source: "+a.overview.source)
		} else {
			ui.Text(c, "Config source has not been checked.").TextColor(theme.TextMuted)
		}
		ui.Text(c, "Active Profile").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
		activeProfile := currentOverviewProfile(a.overview.profiles)
		if activeProfile == nil {
			ui.Text(c, "No profile selected.").TextColor(theme.TextMuted)
		} else {
			ui.Text(c, activeProfile.name)
			if activeProfile.api != "" {
				ui.Text(c, "API: "+activeProfile.api)
			}
			if activeProfile.endpoint != "" {
				ui.Text(c, "Upstream: "+activeProfile.endpoint)
			}
			ui.Text(c, fmt.Sprintf("Models: %d (%d exposed) · %d channel(s)", activeProfile.models, activeProfile.exposed, activeProfile.channels))
		}
		ui.Text(c, "Proxy status").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
		a.proxyControls(c)
		if a.trayMessage != "" {
			ui.Text(c, a.trayMessage).TextColor(theme.Warning).Role(ui.RoleStatus)
		}
		ui.Text(c, "Profiles").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
		filter := ui.SearchField(c, &a.profileQuery).Label("Filter profiles").Width(440)
		if filter.Changed() {
			a.selectedProfile = -1
		}
		profiles := a.visibleProfiles()
		if len(profiles) == 0 {
			a.selectedProfile = -1
		} else if a.selectedProfile < 0 || a.selectedProfile >= len(profiles) {
			a.selectedProfile = 0
		}
		a.profileList.Key = func(i int) any { return profiles[i].name }
		a.profileList.Label = func(i int) string {
			label := profiles[i].name
			if profiles[i].current {
				label += " (current)"
			}
			return label
		}
		a.profileList.Selected = &a.selectedProfile
		selectedName := ""
		selectedIsCurrent := false
		if a.selectedProfile >= 0 && a.selectedProfile < len(profiles) {
			selectedName = profiles[a.selectedProfile].name
			selectedIsCurrent = profiles[a.selectedProfile].current
		}
		ui.Row(c).HandleInput(blockComposingPress(filter.Composing())).Children(func() {
			if ui.PrimaryButton(c, "Switch to selected profile").Disabled(filter.Composing() || a.overviewBusy || a.profileSwitching || selectedName == "" || selectedIsCurrent).Clicked() {
				a.switchProfile(selectedName)
			}
		})
		if a.profileSwitchMessage != "" {
			ui.Text(c, a.profileSwitchMessage).Role(ui.RoleStatus)
		}
		ui.List(c, &a.profileList, len(profiles), func(i int) {
			label := profiles[i].name
			if profiles[i].current {
				label += " (current)"
			}
			ui.Text(c, label).Padding(6, 8)
		}).Label("Profiles").MinHeight(120).Grow(1).Children(func() {
			if len(profiles) == 0 {
				message := "No profiles configured."
				if a.profileQuery != "" {
					message = "No profiles match the filter."
				}
				ui.Text(c, message).TextColor(theme.TextMuted).Padding(12)
			}
		})
		ui.Text(c, "Settings summary").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
		if a.overviewError != "" {
			ui.Text(c, a.overviewError).TextColor(theme.Warning).Role(ui.RoleStatus)
		} else if a.overviewBusy {
			ui.Text(c, "Loading settings…").TextColor(theme.TextMuted).Role(ui.RoleStatus)
		} else {
			ui.Text(c, "Proxy: "+a.overview.proxyAddress)
			ui.Text(c, "Web UI: "+a.overview.webAddress)
		}
		if ui.Button(c, "Refresh Overview").Clicked() {
			a.refreshNativeOverview()
			a.refreshProxy()
		}
	})
}

func currentOverviewProfile(profiles []overviewProfile) *overviewProfile {
	for i := range profiles {
		if profiles[i].current {
			return &profiles[i]
		}
	}
	return nil
}

func (a *desktopShell) visibleProfiles() []overviewProfile {
	query := strings.ToLower(strings.TrimSpace(a.profileQuery))
	if query == "" {
		return a.overview.profiles
	}
	var filtered []overviewProfile
	for _, profile := range a.overview.profiles {
		if strings.Contains(strings.ToLower(profile.name), query) {
			filtered = append(filtered, profile)
		}
	}
	return filtered
}
