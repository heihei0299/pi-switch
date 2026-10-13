package main

import (
	"errors"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/packages"
)

type nativePackage struct {
	id           string
	typeName     string
	name         string
	version      string
	enabled      bool
	capabilities []string
}

type nativePackagesState struct {
	packages             []nativePackage
	packagesLoaded       bool
	packagesLoading      bool
	packagesError        string
	packagesMessage      string
	packageSpec          string
	packageList          ui.ListState
	packageSelected      int
	packageDeleteConfirm bool
	packageDeleteID      string
	packageDeleteName    string
	runPackageTask       func(func() (string, error))
}

func loadNativePackages() ([]nativePackage, error) {
	if _, err := os.Stat(packages.PiSwitchDBPath()); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	rows, err := packages.ListInstalledPackages()
	if err != nil {
		return nil, err
	}
	packages := make([]nativePackage, 0, len(rows))
	for _, row := range rows {
		pkg := nativePackage{
			id:       nativePackageString(row, "id"),
			typeName: nativePackageString(row, "type"),
			name:     nativePackageString(row, "name"),
			version:  nativePackageString(row, "version"),
			enabled:  nativePackageBool(row, "enabled"),
		}
		for _, capability := range []struct {
			key   string
			label string
		}{{"hasExtensions", "extensions"}, {"hasSkills", "skills"}, {"hasPrompts", "prompts"}, {"hasThemes", "themes"}} {
			if nativePackageBool(row, capability.key) {
				pkg.capabilities = append(pkg.capabilities, capability.label)
			}
		}
		packages = append(packages, pkg)
	}
	return packages, nil
}
func nativePackageString(row map[string]interface{}, key string) string {
	value, _ := row[key].(string)
	return value
}
func nativePackageBool(row map[string]interface{}, key string) bool {
	value, _ := row[key].(bool)
	return value
}
func nativePackageDisplayName(pkg nativePackage) string {
	if parsed, err := url.Parse(pkg.name); err == nil && parsed.Host != "" {
		name := path.Base(strings.TrimSuffix(parsed.Path, ".git"))
		if name != "." && name != "/" {
			return name
		}
		return parsed.Hostname()
	}
	if pkg.typeName == "local" {
		return filepath.Base(pkg.name)
	}
	return pkg.name
}
func (a *nativePackagesState) packagesSection(c *ui.Context) {
	ui.Text(c, "Packages · local package registry").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
	if !a.packagesLoaded && !a.packagesLoading && a.runPackageTask != nil {
		a.runPackageTask(func() (string, error) { return "", nil })
	}
	if a.packagesLoading {
		ui.Text(c, "Loading packages…").Role(ui.RoleStatus)
	}
	if a.packagesError != "" {
		ui.Text(c, a.packagesError).TextColor(c.Theme().Warning).Role(ui.RoleStatus)
	}
	if a.packagesMessage != "" {
		ui.Text(c, a.packagesMessage).Role(ui.RoleStatus)
	}
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, "Refresh packages").Disabled(a.packagesLoading || a.runPackageTask == nil).Clicked() && a.runPackageTask != nil {
			a.runPackageTask(func() (string, error) { return "", nil })
		}
		if ui.Button(c, "Import from Pi Agent").Disabled(a.packagesLoading || a.runPackageTask == nil).Clicked() && a.runPackageTask != nil {
			a.runPackageTask(func() (string, error) {
				result, err := packages.ImportPiPackages()
				if err != nil {
					return "", err
				}
				return result.Message, nil
			})
		}
	})
	ui.Row(c).Gap(8).Children(func() {
		ui.TextInput(c, &a.packageSpec).Label("Package spec").Width(440)
		if ui.PrimaryButton(c, "Register package").Disabled(a.packagesLoading || a.runPackageTask == nil || strings.TrimSpace(a.packageSpec) == "").Clicked() && a.runPackageTask != nil {
			spec := strings.TrimSpace(a.packageSpec)
			a.runPackageTask(func() (string, error) {
				if err := packages.AddInstalledPackage(spec, true); err != nil {
					return "", err
				}
				return "Package registered with pi-switch", nil
			})
		}
	})
	if !a.packagesLoaded && !a.packagesLoading && a.runPackageTask == nil {
		ui.Text(c, "Package service is unavailable.").TextColor(c.Theme().TextMuted)
	} else if a.packagesLoaded && len(a.packages) == 0 {
		ui.Text(c, "No packages registered.").TextColor(c.Theme().TextMuted)
	} else if len(a.packages) > 0 {
		a.packageList.Key = func(i int) any { return a.packages[i].id }
		a.packageList.Label = func(i int) string { return nativePackageDisplayName(a.packages[i]) }
		a.packageList.Selected = &a.packageSelected
		ui.List(c, &a.packageList, len(a.packages), func(i int) {
			pkg := a.packages[i]
			name := nativePackageDisplayName(pkg)
			ui.Row(c).Gap(10).Children(func() {
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, name).Bold()
					ui.Text(c, pkg.typeName+" · v"+pkg.version).TextColor(c.Theme().TextMuted)
					if len(pkg.capabilities) > 0 {
						ui.Text(c, "Capabilities: "+strings.Join(pkg.capabilities, ", ")).TextColor(c.Theme().TextMuted)
					}
				})
				label := "Enable " + name
				if pkg.enabled {
					label = "Disable " + name
				}
				if ui.Button(c, label).Disabled(a.packagesLoading || a.runPackageTask == nil).Clicked() && a.runPackageTask != nil {
					id := pkg.id
					a.runPackageTask(func() (string, error) {
						enabled, err := packages.ToggleInstalledPackage(id)
						if err != nil {
							return "", err
						}
						if enabled {
							return "Enabled package " + name, nil
						}
						return "Disabled package " + name, nil
					})
				}
				if ui.Button(c, "Remove registration for "+name).Disabled(a.packagesLoading || a.runPackageTask == nil).Clicked() {
					a.packageDeleteID = pkg.id
					a.packageDeleteName = name
					a.packageDeleteConfirm = true
				}
			})
		}).Label("Registered packages").Height(280)
	}
	if ui.AlertDialog(c, &a.packageDeleteConfirm, "Remove package registration?",
		"Remove "+a.packageDeleteName+" from the pi-switch package registry?",
		"Cancel", "Remove registration") == 1 && a.runPackageTask != nil {
		id, name := a.packageDeleteID, a.packageDeleteName
		a.runPackageTask(func() (string, error) {
			if err := packages.DeleteInstalledPackage(id); err != nil {
				return "", err
			}
			return "Removed registration for " + name, nil
		})
	}
}

func (p *nativePackagesState) runTask(shell *desktopShell, task func() (string, error)) {
	if shell.window == nil || p.packagesLoading || shell.exiting {
		return
	}
	preview := p
	window := shell.window
	ctx, token, ok := shell.tasks.begin("packages")
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
			if window.IsDestroyed() || !shell.tasks.accept("packages", token) {
				shell.tasks.finish("packages", token)
				return
			}
			defer shell.tasks.finish("packages", token)
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
