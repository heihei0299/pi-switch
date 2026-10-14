package main

import (
	"context"
	"encoding/json"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/bootstrap"
	"github.com/heihei0299/pi-switch/internal/config"
	settingsusecase "github.com/heihei0299/pi-switch/internal/usecase/settings"
)

type nativeSettingsPage struct {
	draft, revision, message         string
	loaded, busy, confirm, composing bool
}

func (p *nativeSettingsPage) load(shell *desktopShell) {
	if p.busy {
		return
	}
	p.busy = shell.runStandardTask("settings", func() (interface{}, error) {
		return bootstrap.NewSettingsService(config.ResolvePath()).LoadDraft(context.Background())
	}, func(result interface{}, err error) {
		p.busy = false
		if err != nil {
			p.message = nativeManagementError(err)
			return
		}
		draft := result.(settingsusecase.Draft)
		p.draft, p.revision, p.message = draft.JSON, draft.Revision, "Settings loaded. Proxy will not be restarted automatically."
		p.loaded = true
	})
}

func (p *nativeSettingsPage) save(shell *desktopShell) {
	raw, expected := p.draft, p.revision
	p.busy = shell.runStandardTask("settings", func() (interface{}, error) {
		return bootstrap.NewSettingsService(config.ResolvePath()).SaveDraft(context.Background(), raw, expected)
	}, func(result interface{}, err error) {
		p.busy = false
		if err != nil {
			p.message = nativeManagementError(err)
			return
		}
		saved := result.(settingsusecase.Draft)
		p.revision, p.draft = saved.Revision, saved.JSON
		p.message = "Settings saved with a pre-write backup. Restart Proxy explicitly to apply listener changes."
		shell.refreshNativeOverview()
	})
}

func (p *nativeSettingsPage) view(shell *desktopShell, c *ui.Context) {
	ui.Column(c).Padding(24).Gap(12).Children(func() {
		ui.Text(c, "Settings").FontSize(26).Bold().Role(ui.RoleHeading).Level(1)
		ui.Text(c, nativeBuildIdentityText())
		ui.Text(c, "All existing settings are editable below. Save validates retry rules and ports and backs up the previous config.")
		if ui.Button(c, "Load / discard settings draft").Disabled(p.busy).Clicked() {
			p.confirm = false
			p.load(shell)
		}
		if !p.loaded {
			ui.Text(c, "Settings have not loaded yet.").Role(ui.RoleStatus)
		}
		editor := ui.TextArea(c, &p.draft).Label("Settings JSON").Height(380).Fill().Disabled(p.busy || !p.loaded || p.confirm)
		p.composing = editor.Composing()
		ui.Row(c).HandleInput(blockComposingPress(p.composing)).Children(func() {
			if ui.PrimaryButton(c, "Save settings…").Disabled(p.busy || !p.loaded || p.composing || !json.Valid([]byte(p.draft))).Clicked() {
				p.confirm = true
			}
		})
		if ui.AlertDialog(c, &p.confirm, "Save settings?", "Replace settings after checking for concurrent changes? A pre-write backup will be created. Proxy stays running.", "Cancel", "Save") == 1 {
			p.save(shell)
		}
		if p.busy {
			ui.Text(c, "Settings operation in progress…").Role(ui.RoleStatus)
		}
		if p.message != "" {
			ui.Text(c, p.message).Role(ui.RoleStatus)
		}
		if p.loaded && !json.Valid([]byte(p.draft)) {
			ui.Text(c, "Invalid JSON; draft retained. Nothing has been saved.").TextColor(c.Theme().Warning)
		}
	})
}
