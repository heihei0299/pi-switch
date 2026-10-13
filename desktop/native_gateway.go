package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/application"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/gateway"
)

type nativeGatewayPreview struct {
	currentProviders   []string
	proposedProviders  []string
	candidates         []nativeGatewayCandidate
	removed            []string
	selectedCount      int
	proposedModelCount int
	pendingCount       int
	conflicts          []string
	conflictCount      int
	diagnosticCount    int
}
type nativeGatewayCandidate struct {
	key       string
	label     string
	selection gateway.GatewaySelection
	status    string
	selected  bool
}

type nativeGatewayState struct {
	gateway                  nativeGatewayPreview
	gatewayLoaded            bool
	gatewayLoading           bool
	gatewayError             string
	gatewayList              ui.ListState
	gatewaySelectionExplicit bool
	refreshGateway           func([]gateway.GatewaySelection)
	publishGateway           func([]gateway.GatewaySelection)
	gatewayPublishConfirm    bool
	gatewayPublishing        bool
	gatewayPublishMessage    string
}

func loadNativeGatewayPreview() (nativeGatewayPreview, error) {
	return loadNativeGatewayPreviewForSelection(nil)
}
func loadNativeGatewayPreviewForSelection(selections []gateway.GatewaySelection) (nativeGatewayPreview, error) {
	preview, _, err := loadNativeGatewaySelectionPlan(selections)
	return preview, err
}
func loadNativeGatewaySelectionPlan(selections []gateway.GatewaySelection) (nativeGatewayPreview, gateway.CanonicalGatewayPlan, error) {
	cfg, _, err := config.LoadConfigAtPath(config.ResolvePath())
	if err != nil {
		return nativeGatewayPreview{}, gateway.CanonicalGatewayPlan{}, err
	}
	current, err := application.ReadCurrentGateway()
	if err != nil {
		return nativeGatewayPreview{}, gateway.CanonicalGatewayPlan{}, err
	}
	selectedPlan, err := application.BuildSelectedGatewayPlan(cfg, current, selections)
	if err != nil {
		return nativeGatewayPreview{}, gateway.CanonicalGatewayPlan{}, err
	}
	selected := selectedPlan.Selected
	selectedSet := make(map[gateway.GatewaySelection]bool, len(selected))
	for _, selection := range selected {
		selectedSet[selection] = true
	}
	generated := selectedPlan.Generated
	plan := selectedPlan.Plan
	preview := nativeGatewayPreview{
		currentProviders:   gatewayProviderNames(plan.Current),
		proposedProviders:  gatewayProviderNames(plan.Proposed),
		removed:            plan.PreviewRemoved,
		selectedCount:      len(selected),
		proposedModelCount: len(selectedSet),
		pendingCount:       plan.PendingCount,
		conflicts:          plan.Conflicts,
		conflictCount:      len(plan.Conflicts),
		diagnosticCount:    len(plan.Diagnostics),
	}
	for _, group := range generated.Groups {
		for _, model := range group.Models {
			selection := gateway.GatewaySelection{Supplier: group.Supplier, Channel: group.Channel, Model: model.ID}
			preview.candidates = append(preview.candidates, nativeGatewayCandidate{
				key:       group.Supplier + "\x00" + group.Channel + "\x00" + model.ID,
				label:     fmt.Sprintf("%s / %s / %s · %s", group.Supplier, group.Channel, model.ID, model.Status),
				selection: selection,
				status:    model.Status,
				selected:  selectedSet[selection],
			})
		}
	}
	return preview, plan, nil
}
func publishNativeGatewaySelection(selections []gateway.GatewaySelection) (nativeGatewayPreview, error) {
	_, plan, err := loadNativeGatewaySelectionPlan(selections)
	if err != nil {
		return nativeGatewayPreview{}, err
	}
	if len(plan.Conflicts) > 0 {
		return nativeGatewayPreview{}, fmt.Errorf("gateway plan rejected: %s", strings.Join(plan.Conflicts, "; "))
	}
	if plan.PendingCount == 0 {
		return nativeGatewayPreview{}, fmt.Errorf("gateway has no changes to publish")
	}
	if err := application.PublishGatewayPlan(plan); err != nil {
		return nativeGatewayPreview{}, err
	}
	return loadNativeGatewayPreviewForSelection(selections)
}
func gatewayProviderNames(document map[string]interface{}) []string {
	providers, _ := document["providers"].(map[string]interface{})
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func (a *nativeGatewayState) gatewayDiff(c *ui.Context) {
	ui.Text(c, "Current vs proposed Gateway").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
	if a.refreshGateway != nil {
		if ui.Button(c, "Refresh Gateway Preview").Disabled(a.gatewayLoading || a.gatewayPublishing).Clicked() {
			a.refreshGateway(nativeGatewaySelections(a))
		}
	}
	ui.Text(c, "Model selection previews the shared plan; publishing requires explicit confirmation.").TextColor(c.Theme().TextMuted)
	if a.gatewayLoading {
		ui.Text(c, "Loading Gateway preview…").Role(ui.RoleStatus)
	}
	if a.gatewayError != "" {
		ui.Text(c, a.gatewayError).Role(ui.RoleStatus)
	}
	if a.gatewayPublishMessage != "" {
		ui.Text(c, a.gatewayPublishMessage).Role(ui.RoleStatus)
	}
	if a.gatewayPublishing {
		ui.Text(c, "Publishing Gateway models…").Role(ui.RoleStatus)
	}
	if !a.gatewayLoaded {
		ui.Text(c, "Refresh to load the current configuration.").TextColor(c.Theme().TextMuted)
		return
	}
	ui.Text(c, "Current: "+joinNativeNames(a.gateway.currentProviders))
	ui.Text(c, "Proposed: "+joinNativeNames(a.gateway.proposedProviders))
	ui.Text(c, fmt.Sprintf("Selected models: %d / %d · proposed: %d · pending: %d · conflicts: %d · diagnostics: %d",
		a.gateway.selectedCount, len(a.gateway.candidates), a.gateway.proposedModelCount, a.gateway.pendingCount, a.gateway.conflictCount, a.gateway.diagnosticCount))
	if len(a.gateway.candidates) == 0 {
		ui.Text(c, "No exposed Gateway models.").TextColor(c.Theme().TextMuted)
	} else {
		a.gatewayList.Key = func(i int) any { return a.gateway.candidates[i].key }
		a.gatewayList.Label = func(i int) string { return a.gateway.candidates[i].label }
		ui.List(c, &a.gatewayList, len(a.gateway.candidates), func(i int) {
			candidate := &a.gateway.candidates[i]
			if ui.Checkbox(c, &candidate.selected, candidate.label).Disabled(a.gatewayLoading || a.gatewayPublishing).Changed() {
				a.gatewaySelectionExplicit = true
				if a.refreshGateway != nil {
					a.refreshGateway(nativeGatewaySelections(a))
				}
			}
		}).Label("Gateway model selection").Height(220)
	}
	for _, model := range a.gateway.removed {
		ui.Text(c, "Removed from proposal: "+model).TextColor(c.Theme().TextMuted)
	}
	for _, conflict := range a.gateway.conflicts {
		ui.Text(c, "Conflict: "+conflict).TextColor(c.Theme().Warning)
	}
	if ui.PrimaryButton(c, "Publish selected models").Disabled(a.publishGateway == nil || a.gatewayLoading || a.gatewayPublishing || a.gateway.pendingCount == 0 || a.gateway.conflictCount > 0).Clicked() {
		a.gatewayPublishConfirm = true
	}
	if ui.AlertDialog(c, &a.gatewayPublishConfirm, "Publish Gateway changes?",
		fmt.Sprintf("Write %d selected model(s) to %s? Third-party providers are preserved.", a.gateway.selectedCount, gateway.ModelsPath()),
		"Cancel", "Publish") == 1 && a.publishGateway != nil {
		a.gatewayPublishMessage = ""
		a.publishGateway(nativeGatewaySelections(a))
	}
}
func nativeGatewaySelections(preview *nativeGatewayState) []gateway.GatewaySelection {
	if !preview.gatewaySelectionExplicit {
		return nil
	}
	selected := make([]gateway.GatewaySelection, 0, preview.gateway.selectedCount)
	for _, candidate := range preview.gateway.candidates {
		if candidate.selected {
			selected = append(selected, candidate.selection)
		}
	}
	return selected
}
func joinNativeNames(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return fmt.Sprint(names)
}
