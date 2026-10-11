package main

import (
	"encoding/json"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/application"
	"github.com/heihei0299/pi-switch/internal/config"
)

type nativeProvidersPage struct {
	query, original, name, draft, revision, message         string
	key, keyChannel, fetchChannel, preset, copyName, action string
	editing, busy, confirm, composing                       bool
	selected                                                int
	list                                                    ui.ListState
	usage                                                   []application.ProviderUsageWindow
}

func (p *nativeProvidersPage) cancelDraft() {
	p.original, p.name, p.draft, p.revision, p.key, p.keyChannel = "", "", "", "", "", ""
	p.fetchChannel, p.copyName, p.action = "", "", ""
	p.usage = nil
	p.editing, p.confirm = false, false
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

func (a *desktopShell) loadNativeProvider(name string) {
	p := &a.standardPages().providers
	if p.busy {
		return
	}
	p.busy = a.runStandardTask("providers", func() (interface{}, error) {
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

func (a *desktopShell) saveNativeProvider() {
	p := &a.standardPages().providers
	original, name, raw, expected, key, channel := p.original, p.name, p.draft, p.revision, p.key, p.keyChannel
	p.busy = a.runStandardTask("providers", func() (interface{}, error) {
		prof, err := application.ProviderFromDraft(config.ResolvePath(), original, raw, expected, map[string]string{channel: key})
		if err != nil {
			return nil, err
		}
		return nil, application.SaveProvider(config.ResolvePath(), original, name, prof, expected)
	}, func(_ interface{}, err error) {
		p.busy = false
		if err != nil {
			p.message = nativeManagementError(err)
			return
		}
		p.cancelDraft()
		p.message = "Provider saved with a pre-write backup. Gateway was not published."
		a.refreshNativeOverview()
	})
}

func (a *desktopShell) deleteNativeProvider() {
	p := &a.standardPages().providers
	name, expected := p.original, p.revision
	p.busy = a.runStandardTask("providers", func() (interface{}, error) {
		return nil, application.DeleteProvider(config.ResolvePath(), name, expected)
	}, func(_ interface{}, err error) {
		p.busy = false
		if err != nil {
			p.message = nativeManagementError(err)
			return
		}
		p.cancelDraft()
		p.message = "Provider deleted with a pre-write backup. Existing models.json and running Proxy are unchanged."
		a.refreshNativeOverview()
	})
}

func (a *desktopShell) validateNativeProvider() {
	p := &a.standardPages().providers
	original, raw, expected, key, channel := p.original, p.draft, p.revision, p.key, p.keyChannel
	p.busy = a.runStandardTask("providers", func() (interface{}, error) {
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

func (a *desktopShell) providersView(c *ui.Context) {
	p := &a.standardPages().providers
	ui.Column(c).Padding(24).Gap(12).Children(func() {
		ui.Text(c, a.standardPages().page).FontSize(26).Bold().Role(ui.RoleHeading).Level(1)
		ui.Text(c, "Profiles and Providers share config.json; channels, model metadata and exposure are edited in the same provider draft.")
		if a.overviewError != "" {
			ui.Text(c, a.overviewError).TextColor(c.Theme().Warning).Role(ui.RoleStatus)
		}
		if ui.SearchField(c, &p.query).Label("Find provider").Disabled(p.busy || p.confirm).Changed() {
			p.selected = -1
		}
		var names []string
		for _, prof := range a.overview.profiles {
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
		locked := p.busy || p.confirm || p.editing
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "Add provider").Disabled(locked).Clicked() {
				p.newDraft()
			}
			if ui.Button(c, "Edit selected provider").Disabled(locked || selected == "").Clicked() {
				a.loadNativeProvider(selected)
			}
			if ui.Button(c, "Switch selected profile").Disabled(locked || selected == "" || a.profileSwitching || a.overviewBusy).Clicked() {
				a.switchProfile(selected)
			}
			if ui.Button(c, "Refresh providers").Disabled(p.busy).Clicked() {
				a.refreshNativeOverview()
			}
		})
		if !p.editing {
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
		if a.profileSwitchMessage != "" {
			ui.Text(c, a.profileSwitchMessage).Role(ui.RoleStatus)
		}
		if p.editing {
			nameInput := ui.TextInput(c, &p.name).Label("Provider name").Disabled(p.busy || p.confirm)
			ui.Text(c, "Existing keys, header values and stored URLs are hidden. Use the password field to replace one API key without rendering it.")
			channelInput := ui.TextInput(c, &p.keyChannel).Label("Key channel (empty = profile-level key)").Disabled(p.busy || p.confirm)
			keyInput := ui.TextInput(c, &p.key).Password().Label("Replacement API key (empty = unchanged)").Disabled(p.busy || p.confirm)
			editor := ui.TextArea(c, &p.draft).Label("Provider / channels / models JSON").Height(360).Fill().Disabled(p.busy || p.confirm)
			p.composing = nameInput.Composing() || channelInput.Composing() || keyInput.Composing() || editor.Composing()
			if !json.Valid([]byte(p.draft)) {
				ui.Text(c, "Invalid JSON; draft retained and Save disabled.").TextColor(c.Theme().Warning)
			}
			a.providerNetworkControls(c)
			ui.Row(c).Gap(8).HandleInput(blockComposingPress(p.composing)).Children(func() {
				if ui.Button(c, "Cancel provider draft").Disabled(p.busy || p.confirm).Clicked() {
					p.cancelDraft()
					p.message = "Draft discarded. No config or backup was written."
				}
				if ui.Button(c, "Validate provider").Disabled(p.busy || p.confirm || p.composing).Clicked() {
					a.validateNativeProvider()
				}
				if ui.PrimaryButton(c, "Save provider…").Disabled(p.busy || p.confirm || p.composing || strings.TrimSpace(p.name) == "" || !json.Valid([]byte(p.draft))).Clicked() {
					p.action, p.confirm = "save", true
				}
				if ui.Button(c, "Delete provider…").Disabled(p.busy || p.confirm || p.composing || p.original == "").Clicked() {
					p.action, p.confirm = "delete", true
				}
			})
		}
		if p.busy {
			ui.Text(c, "Provider operation in progress…").Role(ui.RoleStatus)
		}
		if p.message != "" {
			ui.Text(c, p.message).Role(ui.RoleStatus)
		}
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
				a.deleteNativeProvider()
			case "duplicate":
				a.duplicateNativeProvider()
			default:
				a.saveNativeProvider()
			}
		}
	})
}
