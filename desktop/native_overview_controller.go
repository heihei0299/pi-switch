package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

type nativeOverviewController struct {
	snapshot       nativeOverview
	error          string
	busy           bool
	refreshPending bool
	profileQuery   string
	profileList    ui.ListState
	selected       int
	switching      bool
	switchMessage  string
}

func (c *nativeOverviewController) refresh(shell *desktopShell) {
	if shell.window == nil || shell.exiting {
		return
	}
	if c.busy || c.switching {
		c.refreshPending = true
		return
	}
	c.busy = true
	ctx, token, ok := shell.tasks.begin("overview")
	if !ok {
		c.busy = false
		return
	}
	shell.window.Invalidate()
	window := shell.window
	go func() {
		overview, err := loadNativeOverview()
		if ctx.Err() != nil {
			return
		}
		window.Update(func() {
			if window.IsDestroyed() || !shell.tasks.accept("overview", token) {
				shell.tasks.finish("overview", token)
				return
			}
			shell.tasks.finish("overview", token)
			c.update(overview, err)
			c.flushRefresh(shell)
		})
	}()
}

func (c *nativeOverviewController) flushRefresh(shell *desktopShell) {
	if !c.refreshPending {
		return
	}
	c.refreshPending = false
	c.refresh(shell)
}

func (c *nativeOverviewController) update(overview nativeOverview, err error) {
	c.busy = false
	if err != nil {
		c.error = err.Error()
		return
	}
	c.snapshot = overview
	c.error = ""
	c.selected = -1
}

func (c *nativeOverviewController) switchProfile(shell *desktopShell, name string) {
	if c.switching || shell.exiting || shell.window == nil {
		return
	}
	c.switching = true
	ctx, token, ok := shell.tasks.begin("overview")
	if !ok {
		c.switching = false
		return
	}
	c.switchMessage = "Switching profile…"
	window := shell.window
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
			if window.IsDestroyed() || !shell.tasks.accept("overview", token) {
				shell.tasks.finish("overview", token)
				return
			}
			shell.tasks.finish("overview", token)
			c.switching = false
			defer c.flushRefresh(shell)
			if err != nil {
				c.switchMessage = "Switch failed: " + nativeManagementError(err)
				return
			}
			c.switchMessage = "Switched to " + name
			if overviewErr != nil {
				c.error = overviewErr.Error()
				return
			}
			c.snapshot = overview
			c.error = ""
		})
	}()
}

func (c *nativeOverviewController) visibleProfiles() []overviewProfile {
	query := strings.ToLower(strings.TrimSpace(c.profileQuery))
	if query == "" {
		return c.snapshot.profiles
	}
	var filtered []overviewProfile
	for _, profile := range c.snapshot.profiles {
		if strings.Contains(strings.ToLower(profile.name), query) {
			filtered = append(filtered, profile)
		}
	}
	return filtered
}

func (c *nativeOverviewController) currentProfile() *overviewProfile {
	for i := range c.snapshot.profiles {
		if c.snapshot.profiles[i].current {
			return &c.snapshot.profiles[i]
		}
	}
	return nil
}

func (c *nativeOverviewController) view(shell *desktopShell, context *ui.Context) {
	theme := context.Theme()
	ui.Column(context).Fill().Padding(24).Gap(12).Children(func() {
		ui.Text(context, "pi-switch").FontSize(26).Bold().Role(ui.RoleHeading).Level(1)
		ui.Text(context, "Native management").TextColor(theme.TextMuted)
		if c.snapshot.source != "" {
			ui.Text(context, "Config source: "+c.snapshot.source)
		} else {
			ui.Text(context, "Config source has not been checked.").TextColor(theme.TextMuted)
		}
		ui.Text(context, "Active Profile").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
		activeProfile := c.currentProfile()
		if activeProfile == nil {
			ui.Text(context, "No profile selected.").TextColor(theme.TextMuted)
		} else {
			ui.Text(context, activeProfile.name)
			if activeProfile.api != "" {
				ui.Text(context, "API: "+activeProfile.api)
			}
			if activeProfile.endpoint != "" {
				ui.Text(context, "Upstream: "+activeProfile.endpoint)
			}
			ui.Text(context, fmt.Sprintf("Models: %d (%d exposed) · %d channel(s)", activeProfile.models, activeProfile.exposed, activeProfile.channels))
		}
		ui.Text(context, "Proxy status").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
		shell.proxy.controls(shell, context)
		if shell.trayMessage != "" {
			ui.Text(context, shell.trayMessage).TextColor(theme.Warning).Role(ui.RoleStatus)
		}
		ui.Text(context, "Profiles").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
		filter := ui.SearchField(context, &c.profileQuery).Label("Filter profiles").Width(440)
		if filter.Changed() {
			c.selected = -1
		}
		profiles := c.visibleProfiles()
		if len(profiles) == 0 {
			c.selected = -1
		} else if c.selected < 0 || c.selected >= len(profiles) {
			c.selected = 0
		}
		c.profileList.Key = func(i int) any { return profiles[i].name }
		c.profileList.Label = func(i int) string {
			label := profiles[i].name
			if profiles[i].current {
				label += " (current)"
			}
			return label
		}
		c.profileList.Selected = &c.selected
		selectedName := ""
		selectedIsCurrent := false
		if c.selected >= 0 && c.selected < len(profiles) {
			selectedName = profiles[c.selected].name
			selectedIsCurrent = profiles[c.selected].current
		}
		ui.Row(context).HandleInput(blockComposingPress(filter.Composing())).Children(func() {
			if ui.PrimaryButton(context, "Switch to selected profile").Disabled(filter.Composing() || c.busy || c.switching || selectedName == "" || selectedIsCurrent).Clicked() {
				c.switchProfile(shell, selectedName)
			}
		})
		if c.switchMessage != "" {
			ui.Text(context, c.switchMessage).Role(ui.RoleStatus)
		}
		ui.List(context, &c.profileList, len(profiles), func(i int) {
			label := profiles[i].name
			if profiles[i].current {
				label += " (current)"
			}
			ui.Text(context, label).Padding(6, 8)
		}).Label("Profiles").MinHeight(120).Grow(1).Children(func() {
			if len(profiles) == 0 {
				message := "No profiles configured."
				if c.profileQuery != "" {
					message = "No profiles match the filter."
				}
				ui.Text(context, message).TextColor(theme.TextMuted).Padding(12)
			}
		})
		ui.Text(context, "Settings summary").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
		if c.error != "" {
			ui.Text(context, c.error).TextColor(theme.Warning).Role(ui.RoleStatus)
		} else if c.busy {
			ui.Text(context, "Loading settings…").TextColor(theme.TextMuted).Role(ui.RoleStatus)
		} else {
			ui.Text(context, "Proxy: "+c.snapshot.proxyAddress)
			ui.Text(context, "Web UI: "+c.snapshot.webAddress)
		}
		if ui.Button(context, "Refresh Overview").Clicked() {
			c.refresh(shell)
			shell.proxy.refresh(shell)
		}
	})
}

func (c *nativeOverviewController) providerData() ([]overviewProfile, string, bool, bool, string) {
	return c.snapshot.profiles, c.error, c.busy, c.switching, c.switchMessage
}

func (c *nativeOverviewController) proxyData() (string, string) {
	return c.snapshot.proxyAddress, c.error
}
