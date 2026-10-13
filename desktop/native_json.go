package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/application"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/gateway"
)

type nativeJSONState struct {
	jsonDraft           string
	jsonStatus          string
	jsonConflicts       []string
	jsonDraftValidated  bool
	jsonDraftPending    int
	jsonDraftAdded      []string
	jsonDraftRemoved    []string
	jsonDraftChanged    []string
	jsonPublishConfirm  bool
	publishGatewayDraft func(string)
	composing           bool
}

func loadNativeGatewayDraft() (string, error) {
	cfg, _, err := config.LoadConfigAtPath(config.ResolvePath())
	if err != nil {
		return "", err
	}
	encoded, err := json.MarshalIndent(gateway.BuildProposedGatewayEntry(cfg), "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
func nativeGatewayDraftPlan(raw string) (gateway.CanonicalGatewayPlan, error) {
	var draft map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &draft); err != nil {
		return gateway.CanonicalGatewayPlan{}, err
	}
	cfg, _, err := config.LoadConfigAtPath(config.ResolvePath())
	if err != nil {
		return gateway.CanonicalGatewayPlan{}, err
	}
	current, err := gateway.ReadCurrent()
	if err != nil {
		return gateway.CanonicalGatewayPlan{}, err
	}
	plan, _ := application.BuildDraftGatewayPlan(cfg, current, draft)
	return plan, nil
}
func nativeJSONSyntaxErrorOffset(raw string) (int, bool) {
	var value interface{}
	err := json.Unmarshal([]byte(raw), &value)
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		return 0, false
	}
	offset := int(syntaxErr.Offset)
	if err.Error() != "unexpected end of JSON input" {
		offset--
	}
	if offset < 0 {
		offset = 0
	} else if offset > len(raw) {
		offset = len(raw)
	}
	return offset, true
}
func nativeJSONSyntaxLineRange(raw string) (int, int, bool) {
	offset, ok := nativeJSONSyntaxErrorOffset(raw)
	if !ok {
		return 0, 0, false
	}
	lineStart := strings.LastIndex(raw[:offset], "\n") + 1
	lineEnd := len(raw)
	if nextLine := strings.Index(raw[lineStart:], "\n"); nextLine >= 0 {
		lineEnd = lineStart + nextLine
	}
	if lineStart == lineEnd {
		return 0, 0, false
	}
	return utf8.RuneCountInString(raw[:lineStart]), utf8.RuneCountInString(raw[:lineEnd]), true
}
func nativeJSONSyntaxStatus(raw string) string {
	offset, ok := nativeJSONSyntaxErrorOffset(raw)
	if !ok {
		return "Invalid JSON — draft preserved"
	}
	prefix := raw[:offset]
	lineStart := strings.LastIndex(prefix, "\n") + 1
	line := strings.Count(prefix, "\n") + 1
	column := utf8.RuneCountInString(prefix[lineStart:]) + 1
	return fmt.Sprintf("Invalid JSON at line %d, column %d — draft preserved", line, column)
}
func (a *nativeJSONState) invalidateJSONDraftValidation() {
	a.jsonConflicts = nil
	a.jsonDraftValidated = false
	a.jsonDraftPending = 0
	a.jsonDraftAdded = nil
	a.jsonDraftRemoved = nil
	a.jsonDraftChanged = nil
}
func publishNativeGatewayDraft(raw string) (nativeGatewayPreview, error) {
	plan, err := nativeGatewayDraftPlan(raw)
	if err != nil {
		return nativeGatewayPreview{}, err
	}
	if len(plan.Conflicts) > 0 {
		return nativeGatewayPreview{}, fmt.Errorf("gateway draft rejected: %s", strings.Join(plan.Conflicts, "; "))
	}
	if plan.PendingCount == 0 {
		return nativeGatewayPreview{}, fmt.Errorf("gateway draft has no changes to publish")
	}
	if err := application.PublishGatewayPlan(plan); err != nil {
		return nativeGatewayPreview{}, err
	}
	return loadNativeGatewayPreview()
}
func (a *nativeJSONState) editor(c *ui.Context, gatewayState *nativeGatewayState) {
	ui.Text(c, "Gateway JSON draft validation").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
	ui.Text(c, "Load starts from generated pi-switch providers only. Validation is read-only; draft edits are not published.").TextColor(c.Theme().TextMuted)
	editor := ui.TextArea(c, &a.jsonDraft).Label("Draft JSON").Disabled(gatewayState.gatewayPublishing).Height(260).Fill()
	if editor.Changed() {
		a.jsonStatus = ""
		a.invalidateJSONDraftValidation()
	}
	if strings.HasPrefix(a.jsonStatus, "Invalid JSON at") {
		if start, end, ok := nativeJSONSyntaxLineRange(a.jsonDraft); ok {
			editor.TextRanges(ui.TextRange{Start: start, End: end, Color: c.Theme().Danger})
		}
	}
	a.composing = editor.Composing()
	if a.composing {
		ui.Text(c, "Input method composition in progress").TextColor(c.Theme().TextMuted)
	}
	if ui.Button(c, "Load safe Gateway draft").Disabled(gatewayState.gatewayPublishing).Clicked() {
		draft, err := loadNativeGatewayDraft()
		if err != nil {
			a.invalidateJSONDraftValidation()
			a.jsonStatus = "Load failed: " + err.Error()
		} else {
			a.jsonDraft = draft
			a.invalidateJSONDraftValidation()
			a.jsonStatus = "Loaded generated Gateway draft"
		}
	}
	if ui.Button(c, "Validate Gateway draft").Disabled(gatewayState.gatewayPublishing).Clicked() {
		if !json.Valid([]byte(a.jsonDraft)) {
			a.invalidateJSONDraftValidation()
			a.jsonStatus = nativeJSONSyntaxStatus(a.jsonDraft)
		} else {
			plan, err := nativeGatewayDraftPlan(a.jsonDraft)
			if err != nil {
				a.invalidateJSONDraftValidation()
				a.jsonStatus = "Validation failed: " + err.Error()
			} else {
				a.jsonConflicts = plan.Conflicts
				a.jsonDraftPending = plan.PendingCount
				a.jsonDraftAdded = plan.Added
				a.jsonDraftRemoved = plan.Removed
				a.jsonDraftChanged = plan.Changed
				a.jsonDraftValidated = len(plan.Conflicts) == 0
				if len(plan.Conflicts) > 0 {
					a.jsonStatus = fmt.Sprintf("Gateway draft has %d conflict(s)", len(plan.Conflicts))
				} else {
					a.jsonStatus = fmt.Sprintf("Valid Gateway draft · %d pending change(s)", plan.PendingCount)
				}
			}
		}
	}
	if a.jsonStatus != "" {
		ui.Text(c, a.jsonStatus).Role(ui.RoleStatus)
	}
	for _, conflict := range a.jsonConflicts {
		ui.Text(c, "Conflict: "+conflict).TextColor(c.Theme().Warning)
	}
	for _, provider := range a.jsonDraftAdded {
		ui.Text(c, "Added provider: "+provider)
	}
	for _, provider := range a.jsonDraftRemoved {
		ui.Text(c, "Removed provider: "+provider)
	}
	for _, provider := range a.jsonDraftChanged {
		ui.Text(c, "Changed provider: "+provider)
	}
	if ui.PrimaryButton(c, "Publish validated Gateway draft").Disabled(a.publishGatewayDraft == nil || gatewayState.gatewayPublishing || !a.jsonDraftValidated || a.jsonDraftPending == 0).Clicked() {
		a.jsonPublishConfirm = true
	}
	if ui.AlertDialog(c, &a.jsonPublishConfirm, "Publish Gateway draft?",
		fmt.Sprintf("Write this Gateway draft to %s? Third-party providers are preserved.", gateway.ModelsPath()),
		"Cancel", "Publish") == 1 && a.publishGatewayDraft != nil {
		gatewayState.gatewayPublishMessage = ""
		a.publishGatewayDraft(a.jsonDraft)
	}
}
