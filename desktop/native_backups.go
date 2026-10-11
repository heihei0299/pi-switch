package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/application"
	"github.com/heihei0299/pi-switch/internal/config"
)

type nativeBackupsPage struct {
	items                       []application.ConfigBackup
	selected, revision, message string
	busy, loaded, confirm       bool
}

func (a *desktopShell) loadNativeBackups() {
	p := &a.standardPages().backups
	if p.busy {
		return
	}
	p.busy = a.runStandardTask("backups", func() (interface{}, error) {
		revision, err := application.ConfigFileRevision(config.ResolvePath())
		if err != nil {
			return nil, err
		}
		items, err := application.ListConfigBackups(config.ResolvePath())
		return struct {
			items    []application.ConfigBackup
			revision string
		}{items, revision}, err
	}, func(result interface{}, err error) {
		p.busy = false
		if err != nil {
			p.message = nativeManagementError(err)
			return
		}
		data := result.(struct {
			items    []application.ConfigBackup
			revision string
		})
		p.items, p.revision, p.loaded, p.selected, p.confirm = data.items, data.revision, true, "", false
		p.message = "Backups refreshed. Backup contents are not displayed."
	})
}

func (a *desktopShell) backupOperation(restore bool) {
	p := &a.standardPages().backups
	name, expected := p.selected, p.revision
	p.busy = a.runStandardTask("backups", func() (interface{}, error) {
		if restore {
			return nil, application.RestoreConfigBackup(config.ResolvePath(), name, expected)
		}
		name, err := application.CreateConfigBackup(config.ResolvePath())
		if err != nil {
			return nil, err
		}
		if name == "" {
			return "No config file exists; no backup was created.", nil
		}
		return "Backup created: " + name + " (owner-only permissions). Refresh backups to select it.", nil
	}, func(result interface{}, err error) {
		p.busy = false
		if err != nil {
			p.message = nativeManagementError(err)
			return
		}
		if restore {
			p.message = "Config restored. Proxy was not stopped or restarted."
			a.refreshNativeOverview()
		} else {
			p.message = result.(string)
		}
		p.loaded = false
	})
}

func (a *desktopShell) backupsView(c *ui.Context) {
	p := &a.standardPages().backups
	ui.Column(c).Padding(24).Gap(12).Children(func() {
		ui.Text(c, "Backups").FontSize(26).Bold().Role(ui.RoleHeading).Level(1)
		ui.Text(c, "Local snapshots contain credentials, are owner-readable only, and are not encrypted. No export/import or remote sync is provided.")
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "Refresh backups").Disabled(p.busy || p.confirm).Clicked() {
				a.loadNativeBackups()
			}
			if ui.Button(c, "Create backup").Disabled(p.busy || p.confirm).Clicked() {
				a.backupOperation(false)
			}
		})
		if p.loaded && len(p.items) == 0 {
			ui.Text(c, "No backups yet.")
		}
		options := make([]string, len(p.items))
		for i, item := range p.items {
			options[i] = item.Name
		}
		ui.Text(c, "Newest modified snapshots are listed first; select one to inspect its name, time and size before restoring.")
		ui.Select(c, &p.selected, options).Label("Backup to restore").Disabled(p.busy || p.confirm || !p.loaded)
		for _, item := range p.items {
			if item.Name == p.selected {
				ui.Text(c, fmt.Sprintf("Selected snapshot: %s · %d bytes · modified %s", item.Name, item.Size, item.ModifiedAt.Local().Format("2006-01-02 15:04:05 -0700")))
			}
		}
		if ui.Button(c, "Restore selected backup…").Disabled(p.busy || !p.loaded || p.selected == "").Clicked() {
			p.confirm = true
		}
		if ui.AlertDialog(c, &p.confirm, "Restore config backup?", "Replace config.json with "+p.selected+"? The current config is backed up first; a concurrent change aborts the restore. Running Proxy is not restarted.", "Cancel", "Restore") == 1 {
			a.backupOperation(true)
		}
		if p.busy {
			ui.Text(c, "Backup operation in progress…").Role(ui.RoleStatus)
		}
		if p.message != "" {
			ui.Text(c, p.message).Role(ui.RoleStatus)
		}
	})
}
