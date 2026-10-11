package main

import (
	"encoding/json"
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/application"
	"github.com/heihei0299/pi-switch/internal/config"
)

func (p *nativeProvidersPage) newPresetDraft() {
	draft, err := application.ProviderPresetDraft(p.preset)
	if err != nil {
		p.message = nativeManagementError(err)
		return
	}
	p.cancelDraft()
	p.editing, p.draft, p.keyChannel, p.fetchChannel = true, draft.JSON, "primary", "primary"
	p.message = "Preset draft loaded. Credentials are empty and no models are exposed. Validate and save explicitly."
}

func (a *desktopShell) duplicateNativeProvider() {
	p := &a.standardPages().providers
	source, name, expected := p.original, p.copyName, p.revision
	p.busy = a.runStandardTask("providers", func() (interface{}, error) {
		return nil, application.DuplicateProvider(config.ResolvePath(), source, name, expected)
	}, func(_ interface{}, err error) {
		p.busy = false
		if err != nil {
			p.message = nativeManagementError(err)
			return
		}
		p.message = "Saved provider duplicated with a pre-write backup. Unsaved edits were not copied; Gateway and current profile are unchanged."
		a.refreshNativeOverview()
	})
}

func (a *desktopShell) testNativeProvider() {
	p := &a.standardPages().providers
	original, raw, expected, key, channel := p.original, p.draft, p.revision, p.key, p.keyChannel
	p.busy = a.runStandardTask("providers", func() (interface{}, error) {
		return application.TestProviderDraft(config.ResolvePath(), original, raw, expected, map[string]string{channel: key})
	}, func(result interface{}, err error) {
		p.busy = false
		if err != nil {
			p.message = nativeManagementError(err)
			return
		}
		probe := result.(application.ProviderProbe)
		if probe.OK {
			p.message = fmt.Sprintf("Connection OK · %d ms. No config was written.", probe.Milliseconds)
		} else {
			p.message = fmt.Sprintf("Connection failed · %d ms. Upstream response and credentials are hidden; no config was written.", probe.Milliseconds)
		}
	})
}

func (a *desktopShell) fetchNativeProviderModels() {
	p := &a.standardPages().providers
	original, raw, expected, key, channel, fetchChannel := p.original, p.draft, p.revision, p.key, p.keyChannel, p.fetchChannel
	p.busy = a.runStandardTask("providers", func() (interface{}, error) {
		return application.FetchProviderDraftModels(config.ResolvePath(), original, raw, expected, map[string]string{channel: key}, fetchChannel)
	}, func(result interface{}, err error) {
		p.busy = false
		if err != nil {
			p.message = nativeManagementError(err)
			return
		}
		fetched := result.(application.FetchedProviderDraft)
		p.draft = fetched.JSON
		p.message = fmt.Sprintf("Fetched %d model(s) into draft · catalog enriched %d, skipped %d, failed %d. Save explicitly to persist; exposure is unchanged.", fetched.Models, fetched.Counts.Enriched, fetched.Counts.Skipped, fetched.Counts.Failed)
		if fetched.Counts.Warning != "" {
			p.message += " " + fetched.Counts.Warning
		}
	})
}

func (a *desktopShell) refreshNativeProviderUsage() {
	p := &a.standardPages().providers
	name, expected := p.original, p.revision
	p.busy = a.runStandardTask("providers", func() (interface{}, error) {
		return application.LoadProviderUsage(config.ResolvePath(), name, expected)
	}, func(result interface{}, err error) {
		p.busy = false
		if err != nil {
			p.usage = nil
			p.message = nativeManagementError(err)
			return
		}
		p.usage = result.([]application.ProviderUsageWindow)
		p.message = "Usage refreshed for the saved provider's primary upstream; unsaved edits were not used. Unknown usage is not zero."
	})
}

func nativeUsageWindowText(window application.ProviderUsageWindow) string {
	percent := "unknown"
	if window.Percent != nil {
		percent = fmt.Sprintf("%.2f%%", *window.Percent)
	}
	text := window.Name + ": " + percent + " · " + window.Status
	if window.ResetsAt != "" {
		text += " · resets " + window.ResetsAt
	}
	return text
}

func (a *desktopShell) providerNetworkControls(c *ui.Context) {
	p := &a.standardPages().providers
	fetchInput := ui.TextInput(c, &p.fetchChannel).Label("Fetch models for channel").Disabled(p.busy || p.confirm)
	p.composing = p.composing || fetchInput.Composing()
	if p.original != "" {
		copyInput := ui.TextInput(c, &p.copyName).Label("Saved provider copy name").Disabled(p.busy || p.confirm)
		p.composing = p.composing || copyInput.Composing()
	}
	blocked := p.busy || p.confirm || p.composing || !json.Valid([]byte(p.draft))
	ui.Row(c).Gap(8).HandleInput(blockComposingPress(p.composing)).Children(func() {
		if ui.Button(c, "Test draft connection").Disabled(blocked).Clicked() {
			a.testNativeProvider()
		}
		if ui.Button(c, "Refresh saved provider usage").Disabled(p.busy || p.confirm || p.composing || p.original == "").Clicked() {
			a.refreshNativeProviderUsage()
		}
	})
	ui.Row(c).HandleInput(blockComposingPress(p.composing)).Children(func() {
		if ui.Button(c, "Fetch models into draft").Disabled(blocked || p.fetchChannel == "").Clicked() {
			a.fetchNativeProviderModels()
		}
	})
	for _, window := range p.usage {
		ui.Text(c, nativeUsageWindowText(window))
	}
	if p.original != "" {
		ui.Row(c).HandleInput(blockComposingPress(p.composing)).Children(func() {
			if ui.Button(c, "Duplicate saved provider…").Disabled(p.busy || p.confirm || p.composing || p.copyName == "").Clicked() {
				p.action, p.confirm = "duplicate", true
			}
		})
	}
}
