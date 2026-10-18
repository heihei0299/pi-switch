package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/application"
	"github.com/heihei0299/pi-switch/internal/bootstrap"
	"github.com/heihei0299/pi-switch/internal/config"
)

type nativeProvidersPage struct {
	query, original, name, draft, revision, message         string
	key, keyChannel, fetchChannel, preset, copyName, action string
	editing, adding, busy, confirm, composing               bool
	advancedOpen, deleteOpen                                bool
	selected                                                int
	list                                                    ui.ListState
	usage                                                   []application.ProviderUsageWindow
}

func (p *nativeProvidersPage) cancelDraft() {
	p.original, p.name, p.draft, p.revision, p.key, p.keyChannel = "", "", "", "", "", ""
	p.fetchChannel, p.copyName, p.action = "", "", ""
	p.usage = nil
	p.editing, p.adding, p.confirm, p.composing = false, false, false, false
	p.advancedOpen, p.deleteOpen = false, false
}

func (p *nativeProvidersPage) newDraft() {
	p.cancelDraft()
	p.editing = true
	p.keyChannel, p.fetchChannel = "primary", "primary"
	p.draft = `{
  "api": "openai-completions",
  "responsesMode": "auto",
  "baseUrl": "",
  "apiKey": "",
  "upstreams": [{
    "name": "primary",
    "api": "openai-completions",
    "baseUrl": "https://example.test/v1",
    "apiKey": "",
    "models": [],
    "exposedModels": []
  }]
}`
	p.message = "New provider draft. No models are exposed until you explicitly select them."
}

func (p *nativeProvidersPage) load(shell *desktopShell, name string) {
	if p.busy {
		return
	}
	p.busy = shell.runStandardTask("providers", func(_ context.Context) (interface{}, error) {
		return application.LoadProviderDraft(config.ResolvePath(), name)
	}, func(result interface{}, err error) {
		p.busy = false
		if err != nil {
			p.message = nativeManagementError(err)
			return
		}
		draft := result.(application.ProviderDraft)
		p.cancelDraft()
		p.original, p.name, p.draft, p.revision, p.editing = name, name, draft.JSON, draft.Revision, true
		p.copyName, p.fetchChannel = name+"-copy", "main"
		var prof config.ProviderProfile
		if json.Unmarshal([]byte(draft.JSON), &prof) == nil && len(prof.Upstreams) > 0 && prof.ChannelName(0) != "" {
			p.fetchChannel = prof.ChannelName(0)
		}
		p.keyChannel = p.fetchChannel
		p.message = "Loaded a credential-masked draft. KEEP_CREDENTIAL markers preserve existing credentials; removing a marker clears its value."
	})
}

func (p *nativeProvidersPage) save(shell *desktopShell) {
	original, name, raw, expected, key, channel := p.original, p.name, p.draft, p.revision, p.key, p.keyChannel
	p.busy = shell.runStandardTask("providers", func(ctx context.Context) (interface{}, error) {
		prof, err := application.ProviderFromDraft(config.ResolvePath(), original, raw, expected, map[string]string{channel: key})
		if err != nil {
			return nil, err
		}
		return nil, application.SaveProvider(ctx, bootstrap.NewProviderService(config.ResolvePath()), original, name, prof, expected)
	}, func(_ interface{}, err error) {
		p.busy = false
		if err != nil {
			p.message = nativeManagementError(err)
			return
		}
		p.cancelDraft()
		p.message = "Provider saved with a pre-write backup. Gateway was not published."
		shell.refreshNativeOverview()
	})
}

func (p *nativeProvidersPage) delete(shell *desktopShell) {
	name, expected := p.original, p.revision
	p.busy = shell.runStandardTask("providers", func(ctx context.Context) (interface{}, error) {
		return nil, application.DeleteProvider(ctx, bootstrap.NewProviderService(config.ResolvePath()), name, expected)
	}, func(_ interface{}, err error) {
		p.busy = false
		if err != nil {
			p.message = nativeManagementError(err)
			return
		}
		p.cancelDraft()
		p.message = "Provider deleted with a pre-write backup. Existing models.json and running Proxy are unchanged."
		shell.refreshNativeOverview()
	})
}

func (p *nativeProvidersPage) validate(shell *desktopShell) {
	original, raw, expected, key, channel := p.original, p.draft, p.revision, p.key, p.keyChannel
	p.busy = shell.runStandardTask("providers", func(_ context.Context) (interface{}, error) {
		_, err := application.ProviderFromDraft(config.ResolvePath(), original, raw, expected, map[string]string{channel: key})
		return nil, err
	}, func(_ interface{}, err error) {
		p.busy = false
		if err != nil {
			p.message = nativeManagementError(err)
			return
		}
		p.message = "Provider validated by shared profile/channel/model/retry rules. No files were written."
	})
}

func (p *nativeProvidersPage) view(shell *desktopShell, c *ui.Context) {
	profiles, overviewError, overviewBusy, profileSwitching, profileSwitchMessage := shell.overview.providerData()
	ui.Column(c).Padding(24).Gap(12).Children(func() {
		ui.Text(c, shell.standardPages().page).FontSize(26).Bold().Role(ui.RoleHeading).Level(1)
		ui.Text(c, "Profiles and Providers share config.json; channels, model metadata and exposure are edited in the same provider draft.")
		if overviewError != "" {
			ui.Text(c, overviewError).TextColor(c.Theme().Warning).Role(ui.RoleStatus)
		}
		if p.busy {
			ui.Text(c, "Provider operation in progress…").Role(ui.RoleStatus)
		}
		if p.message != "" {
			ui.Text(c, p.message).Role(ui.RoleStatus)
		}
		switch {
		case p.editing:
			p.editView(shell, c)
		case p.adding:
			p.addView(c)
		default:
			p.listView(shell, c, profiles, profileSwitching, profileSwitchMessage, overviewBusy)
		}
		p.confirmDialog(shell, c)
	})
}

// listView finds and selects providers; editing tools only appear in the tasks
// that use them.
func (p *nativeProvidersPage) listView(shell *desktopShell, c *ui.Context, profiles []overviewProfile, profileSwitching bool, profileSwitchMessage string, overviewBusy bool) {
	if ui.SearchField(c, &p.query).Label("Find provider").Disabled(p.busy || p.confirm).Changed() {
		p.selected = -1
	}
	var names []string
	for _, prof := range profiles {
		if strings.Contains(strings.ToLower(prof.name), strings.ToLower(p.query)) {
			names = append(names, prof.name)
		}
	}
	if p.selected >= len(names) {
		p.selected = -1
	}
	p.list.Key = func(i int) any { return names[i] }
	p.list.Label = func(i int) string { return names[i] }
	p.list.Selected = &p.selected
	ui.List(c, &p.list, len(names), func(i int) { ui.Text(c, names[i]).Padding(6, 8) }).Label("Provider list").Height(150)
	if len(names) == 0 {
		ui.Text(c, "No providers match. Add a provider or clear the filter.")
	}
	selected := ""
	if p.selected >= 0 && p.selected < len(names) {
		selected = names[p.selected]
	}
	ui.Row(c).Gap(8).Children(func() {
		if ui.PrimaryButton(c, "Add provider…").Disabled(p.busy || p.confirm).Clicked() {
			p.adding = true
		}
		if ui.Button(c, "Edit selected provider").Disabled(p.busy || p.confirm || selected == "").Clicked() {
			p.load(shell, selected)
		}
		if ui.Button(c, "Switch selected profile").Disabled(p.busy || p.confirm || selected == "" || profileSwitching || overviewBusy).Clicked() {
			shell.overview.switchProfile(shell, selected)
		}
		if ui.Button(c, "Refresh providers").Disabled(p.busy).Clicked() {
			shell.refreshNativeOverview()
		}
	})
	if profileSwitchMessage != "" {
		ui.Text(c, profileSwitchMessage).Role(ui.RoleStatus)
	}
}

// addView is the add task: presets belong here instead of beside the list.
func (p *nativeProvidersPage) addView(c *ui.Context) {
	ui.Text(c, "Add provider").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
	ui.Text(c, "Start from a blank draft or a preset. Nothing is written until you save the draft.").TextColor(c.Theme().TextMuted)
	ui.Row(c).Gap(8).Children(func() {
		if ui.PrimaryButton(c, "Add blank provider").Disabled(p.busy || p.confirm).Clicked() {
			p.newDraft()
		}
		if ui.Button(c, "Cancel add").Disabled(p.busy || p.confirm).Clicked() {
			p.adding = false
		}
	})
	var presets []string
	for _, preset := range application.ProviderPresets() {
		if id, ok := preset["id"].(string); ok {
			presets = append(presets, id)
		}
	}
	ui.Select(c, &p.preset, presets).Label("Provider preset").Disabled(p.busy || p.confirm)
	if ui.Button(c, "Add from preset").Disabled(p.busy || p.confirm || p.preset == "").Clicked() {
		p.newPresetDraft()
	}
}

// editView is the edit task: saving and canceling lead, everything else is
// grouped by what it acts on.
func (p *nativeProvidersPage) editView(shell *desktopShell, c *ui.Context) {
	title := "Editing " + p.original
	if p.original == "" {
		title = "New provider draft"
	}
	ui.Text(c, title).FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
	nameInput := ui.TextInput(c, &p.name).Label("Provider name").Disabled(p.busy || p.confirm)
	ui.Text(c, "Existing keys, header values and stored URLs are hidden. Use the password field to replace one API key without rendering it.")
	channelInput := ui.TextInput(c, &p.keyChannel).Label("Key channel (empty = profile-level key)").Disabled(p.busy || p.confirm)
	keyInput := ui.TextInput(c, &p.key).Password().Label("Replacement API key (empty = unchanged)").Disabled(p.busy || p.confirm)
	editor := ui.TextArea(c, &p.draft).Label("Provider / channels / models JSON").Height(360).Fill().Disabled(p.busy || p.confirm)
	p.composing = nameInput.Composing() || channelInput.Composing() || keyInput.Composing() || editor.Composing()
	if !json.Valid([]byte(p.draft)) {
		ui.Text(c, "Invalid JSON; draft retained and Save disabled.").TextColor(c.Theme().Warning)
	}
	// Auxiliary inputs render below this row, so the guard reads the live
	// composition state instead of the value captured when the row was built.
	ui.Row(c).Gap(8).HandleInput(blockComposingLive(func() bool { return p.composing })).Children(func() {
		if ui.PrimaryButton(c, "Save provider…").Disabled(p.busy || p.confirm || p.composing || strings.TrimSpace(p.name) == "" || !json.Valid([]byte(p.draft))).Clicked() {
			p.action, p.confirm = "save", true
		}
		if ui.Button(c, "Cancel provider draft").Disabled(p.busy || p.confirm).Clicked() {
			p.cancelDraft()
			p.message = "Draft discarded. No config or backup was written."
		}
	})
	ui.Collapsible(c, "More provider actions", &p.advancedOpen, func() {
		p.networkControls(shell, c)
	})
	if p.original != "" {
		ui.Collapsible(c, "Danger zone: delete provider", &p.deleteOpen, func() {
			ui.Text(c, "Deleting removes the saved provider from config.json after a pre-write backup. models.json and a running Proxy are unchanged.")
			// The editor above can hold a preedit; guard the press so a blur
			// commit cannot activate delete.
			ui.Row(c).HandleInput(blockComposingLive(func() bool { return p.composing })).Children(func() {
				if ui.Button(c, "Delete provider…").Disabled(p.busy || p.confirm || p.composing).Clicked() {
					p.action, p.confirm = "delete", true
				}
			})
		})
	}
}

func (p *nativeProvidersPage) confirmDialog(shell *desktopShell, c *ui.Context) {
	title, action := "Save provider?", "Save"
	if p.action == "delete" {
		title, action = "Delete provider?", "Delete"
	}
	if p.action == "duplicate" {
		title, action = "Duplicate saved provider?", "Duplicate"
	}
	message := "Apply this change to config.json after checking for concurrent edits? A pre-write backup is required. Gateway and Proxy are not restarted."
	if p.action == "duplicate" {
		message = "Copy saved provider " + p.original + " to " + p.copyName + "? The copy includes existing credentials. Unsaved edits will not be copied; an existing target is never overwritten."
	}
	if ui.AlertDialog(c, &p.confirm, title, message, "Cancel", action) == 1 {
		switch p.action {
		case "delete":
			p.delete(shell)
		case "duplicate":
			p.duplicate(shell)
		default:
			p.save(shell)
		}
	}
}
