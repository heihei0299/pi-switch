package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/gateway"
	"github.com/heihei0299/pi-switch/internal/server"
	statsservice "github.com/heihei0299/pi-switch/internal/stats"
	"github.com/heihei0299/pi-switch/internal/store"
)

type nativeUsageRow struct {
	name             string
	timestamp        string
	status           string
	promptTokens     *int64
	completionTokens *int64
	cachedTokens     *int64
	reasoningTokens  *int64
	tokens           *int64
	cacheRate        string
	cost             *float64
}

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

type nativeStatsPage struct {
	requestPage        int
	totalRequests      int
	requestTotal       int
	conversationPage   int
	conversationTotal  int
	okRequests         int
	failedRequests     int
	successRate        string
	avgLatencyMs       int64
	totalTokens        map[string]int64
	cacheHitRate       string
	totalCost          *float64
	costUnknown        int64
	conversationSource string
	providerRows       []nativeStatsRow
	modelRows          []nativeStatsRow
	conversations      []nativeConversation
	rows               []nativeUsageRow
}

type nativeStatsRow struct {
	key      string
	label    string
	requests int
}

type nativeConversation struct {
	id       string
	label    string
	requests []string
}

type nativeConversationPage struct {
	id    string
	page  int
	total int
	rows  []string
}

type nativePackage struct {
	id           string
	typeName     string
	name         string
	version      string
	enabled      bool
	capabilities []string
}

type nativeStatsFilters struct {
	requestRange      int
	conversationRange int
	requestFrom       time.Time
	requestTo         time.Time
	conversationFrom  time.Time
	conversationTo    time.Time
}

type nativeComplexPreview struct {
	tab                         int
	jsonDraft                   string
	jsonStatus                  string
	jsonConflicts               []string
	jsonDraftValidated          bool
	jsonDraftPending            int
	jsonDraftAdded              []string
	jsonDraftRemoved            []string
	jsonDraftChanged            []string
	jsonPublishConfirm          bool
	packages                    []nativePackage
	packagesLoaded              bool
	packagesLoading             bool
	packagesError               string
	packagesMessage             string
	packageSpec                 string
	packageList                 ui.ListState
	packageDeleteConfirm        bool
	packageDeleteID             string
	packageDeleteName           string
	runPackageTask              func(func() (string, error))
	composing                   bool
	gateway                     nativeGatewayPreview
	gatewayLoaded               bool
	gatewayLoading              bool
	gatewayError                string
	gatewayList                 ui.ListState
	gatewaySelectionExplicit    bool
	refreshGateway              func([]gateway.GatewaySelection)
	publishGateway              func([]gateway.GatewaySelection)
	publishGatewayDraft         func(string)
	gatewayPublishConfirm       bool
	gatewayPublishing           bool
	gatewayPublishMessage       string
	stats                       nativeStatsPage
	statsLoaded                 bool
	statsLoading                bool
	statsError                  string
	statsFilters                nativeStatsFilters
	refreshStats                func(int, int, nativeStatsFilters)
	conversationRequests        nativeConversationPage
	conversationRequestsLoading bool
	conversationRequestsError   string
	refreshConversationRequests func(string, int)
	usageList                   ui.ListState
	providerList                ui.ListState
	providerChartList           ui.ListState
	modelList                   ui.ListState
	providerSelected            int
	modelSelected               int
	providerChartOpen           bool
	providersOpen               bool
	modelsOpen                  bool
	selectedUsage               int
	sessionOutline              ui.OutlineState[string]
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
	current, err := gateway.ReadCurrent()
	if err != nil {
		return nativeGatewayPreview{}, gateway.CanonicalGatewayPlan{}, err
	}
	generated, _ := gateway.BuildEnrichedGeneratedPlan(cfg, current)
	selected := selections
	if selected == nil {
		selected = make([]gateway.GatewaySelection, 0)
		for _, group := range generated.Groups {
			for _, model := range group.Models {
				if model.Status == "published" {
					selected = append(selected, gateway.GatewaySelection{Supplier: group.Supplier, Channel: group.Channel, Model: model.ID})
				}
			}
		}
	}
	selectedSet := make(map[gateway.GatewaySelection]bool, len(selected))
	for _, selection := range selected {
		selectedSet[selection] = true
	}
	proposed, err := gateway.BuildSelectedGatewayEntry(cfg, selected)
	if err != nil {
		return nativeGatewayPreview{}, gateway.CanonicalGatewayPlan{}, err
	}
	gateway.EnrichDraftModels(cfg, proposed)
	plan := gateway.BuildDraftPlan(cfg, current, proposed)
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
	if err := gateway.PublishPlan(plan); err != nil {
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

const (
	nativeRangeToday = iota
	nativeRange24h
	nativeRange7d
	nativeRangeCustom
	nativeRangeAll
)

func nativeCalendarDay(date time.Time) time.Time {
	local := date.In(time.Local)
	year, month, day := local.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
}

func newNativeStatsFilters(now time.Time) nativeStatsFilters {
	today := nativeCalendarDay(now)
	return nativeStatsFilters{
		requestFrom:      today,
		requestTo:        today,
		conversationFrom: today,
		conversationTo:   today,
	}
}

func nativeStatsWindow(rangeChoice int, fromDate, toDate, now time.Time) (*statsservice.Window, error) {
	if rangeChoice == nativeRangeAll {
		return nil, nil
	}
	var from, to time.Time
	switch rangeChoice {
	case nativeRange24h:
		from = now.Add(-24 * time.Hour)
		to = now
	case nativeRange7d:
		from = now.Add(-7 * 24 * time.Hour)
		to = now
	case nativeRangeCustom:
		if fromDate.IsZero() || toDate.IsZero() {
			return nil, fmt.Errorf("custom range requires start and end dates")
		}
		from = nativeCalendarDay(fromDate)
		end := nativeCalendarDay(toDate)
		if end.Before(from) {
			return nil, fmt.Errorf("end date must be on or after start date")
		}
		to = end.AddDate(0, 0, 1)
	default:
		from = nativeCalendarDay(now)
		to = now
	}
	return &statsservice.Window{From: from.UnixMilli(), To: to.UnixMilli()}, nil
}

func loadNativeStats(requestPage, conversationPage, limit int, filters nativeStatsFilters) (nativeStatsPage, error) {
	now := time.Now()
	requestWindow, err := nativeStatsWindow(filters.requestRange, filters.requestFrom, filters.requestTo, now)
	if err != nil {
		return nativeStatsPage{}, err
	}
	conversationWindow, err := nativeStatsWindow(filters.conversationRange, filters.conversationFrom, filters.conversationTo, now)
	if err != nil {
		return nativeStatsPage{}, err
	}
	if _, err := os.Stat(store.DBPath()); errors.Is(err, os.ErrNotExist) {
		return nativeStatsPage{requestPage: requestPage, conversationPage: conversationPage, successRate: "0%", cacheHitRate: "-"}, nil
	} else if err != nil {
		return nativeStatsPage{}, fmt.Errorf("inspect request history: %w", err)
	}
	service, err := server.OpenStatsService()
	if err != nil {
		return nativeStatsPage{}, err
	}
	result, err := service.Stats(requestWindow, requestPage, limit)
	if err != nil {
		return nativeStatsPage{}, err
	}
	conversations, conversationTotal, err := service.Conversations(conversationWindow, conversationPage, limit)
	if err != nil {
		return nativeStatsPage{}, err
	}
	loaded := nativeStatsPage{
		requestPage:        requestPage,
		totalRequests:      result.TotalRequests,
		requestTotal:       result.RecentRequestTotal,
		conversationPage:   conversationPage,
		conversationTotal:  conversationTotal,
		okRequests:         result.OKRequests,
		failedRequests:     result.FailedRequests,
		successRate:        result.SuccessRate,
		avgLatencyMs:       result.AvgLatencyMs,
		totalTokens:        result.TotalTokens,
		cacheHitRate:       result.CacheHitRate,
		totalCost:          result.TotalCost,
		costUnknown:        result.CostUnknown,
		rows:               make([]nativeUsageRow, 0, len(result.RecentRequests)),
		conversationSource: string(service.Source),
	}
	providerNames := make([]string, 0, len(result.ByProvider))
	for name := range result.ByProvider {
		providerNames = append(providerNames, name)
	}
	sort.Strings(providerNames)
	for _, name := range providerNames {
		stat := result.ByProvider[name]
		loaded.providerRows = append(loaded.providerRows, nativeStatsRow{
			key:      name,
			requests: stat.Total,
			label: fmt.Sprintf("%s · requests: %d · ok: %d · failed: %d · input: %s · output: %s · cached: %s · cache rate: %s · cost: %s",
				name, stat.Total, stat.OK, stat.Failed, nativeTokenDimension(stat.PromptTokens), nativeTokenDimension(stat.OutputTokens),
				nativeTokenDimension(stat.CachedTokens), stat.CacheRate, nativeCostText(stat.Cost)),
		})
	}
	modelNames := make([]string, 0, len(result.ByModel))
	for name := range result.ByModel {
		modelNames = append(modelNames, name)
	}
	sort.Strings(modelNames)
	for _, name := range modelNames {
		stat := result.ByModel[name]
		loaded.modelRows = append(loaded.modelRows, nativeStatsRow{
			key: name,
			label: fmt.Sprintf("%s · requests: %d · ok: %d · input: %s · output: %s · cached: %s · cache rate: %s · cost: %s",
				name, stat.Total, stat.OK, nativeTokenDimension(stat.PromptTokens), nativeTokenDimension(stat.OutputTokens),
				nativeTokenDimension(stat.CachedTokens), stat.CacheRate, nativeCostText(stat.Cost)),
		})
	}
	conversationIndex := make(map[string]int, len(conversations))
	for _, summary := range conversations {
		label := summary.ConversationID
		if summary.Name != nil && *summary.Name != "" {
			label = *summary.Name
		}
		conversationIndex[summary.ConversationID] = len(loaded.conversations)
		loaded.conversations = append(loaded.conversations, nativeConversation{
			id:    summary.ConversationID,
			label: fmt.Sprintf("%s · %d requests", label, summary.Requests),
		})
	}
	for _, row := range result.RecentRequests {
		provider, model := "unknown", "unknown"
		if row.Provider != nil && *row.Provider != "" {
			provider = *row.Provider
		}
		if row.Model != nil && *row.Model != "" {
			model = *row.Model
		}
		timestamp, status := "unknown time", "unknown"
		if row.TS != nil && *row.TS != "" {
			timestamp = *row.TS
		}
		if row.OK != nil {
			status = "failed"
			if *row.OK {
				status = "ok"
			}
		}
		if row.Status != nil {
			status = fmt.Sprintf("%s (%d)", status, *row.Status)
		}
		usage := nativeUsageRow{
			name:             provider + " / " + model,
			timestamp:        timestamp,
			status:           status,
			promptTokens:     row.PromptTokens,
			completionTokens: row.CompletionTokens,
			cachedTokens:     row.CachedTokens,
			reasoningTokens:  row.ReasoningTokens,
			tokens:           row.TotalTokens,
			cacheRate:        row.CacheRate,
			cost:             row.Cost,
		}
		loaded.rows = append(loaded.rows, usage)
		if row.ConversationID != nil {
			if index, ok := conversationIndex[*row.ConversationID]; ok {
				loaded.conversations[index].requests = append(loaded.conversations[index].requests, usageRowText(usage))
			}
		}
	}
	return loaded, nil
}

func loadNativeConversationPage(id string, page, limit int) (nativeConversationPage, error) {
	if _, err := os.Stat(store.DBPath()); errors.Is(err, os.ErrNotExist) {
		return nativeConversationPage{id: id, page: page}, nil
	} else if err != nil {
		return nativeConversationPage{}, fmt.Errorf("inspect request history: %w", err)
	}
	service, err := server.OpenStatsService()
	if err != nil {
		return nativeConversationPage{}, err
	}
	requests, total, err := service.ConversationRequests(id, page, limit)
	if err != nil {
		return nativeConversationPage{}, err
	}
	loaded := nativeConversationPage{id: id, page: page, total: total, rows: make([]string, 0, len(requests))}
	for _, request := range requests {
		loaded.rows = append(loaded.rows, nativeConversationRequestText(request))
	}
	return loaded, nil
}

func nativeConversationRequestText(request statsservice.ConversationRequestDTO) string {
	provider, model, timestamp, status, cost := "unknown", "unknown", "unknown time", "unknown", "unknown"
	if request.Provider != nil && *request.Provider != "" {
		provider = *request.Provider
	}
	if request.Model != nil && *request.Model != "" {
		model = *request.Model
	}
	if request.TS != nil && *request.TS != "" {
		timestamp = *request.TS
	}
	if request.OK != nil {
		status = "failed"
		if *request.OK {
			status = "ok"
		}
	}
	if request.Status != nil {
		status = fmt.Sprintf("%s (%d)", status, *request.Status)
	}
	if request.Cost != nil {
		cost = fmt.Sprintf("$%.2f", *request.Cost)
	}
	cacheRate := request.CacheRate
	if cacheRate == "" {
		cacheRate = "-"
	}
	return fmt.Sprintf("%s · %s / %s · %s · input: %s · output: %s · cached: %s · reasoning: %s · cache rate: %s · total: %s · cost: %s",
		timestamp, provider, model, status,
		nativeTokenText(request.PromptTokens), nativeTokenText(request.CompletionTokens),
		nativeTokenText(request.CachedTokens), nativeTokenText(request.ReasoningTokens), cacheRate,
		nativeTokenText(request.TotalTokens), cost)
}

func nativeTokenText(value *int64) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprint(*value)
}

func newNativeComplexPreview() *nativeComplexPreview {
	preview := &nativeComplexPreview{
		statsFilters: newNativeStatsFilters(time.Now()),
	}
	return preview
}

func loadNativePackages() ([]nativePackage, error) {
	if _, err := os.Stat(server.PiSwitchDBPath()); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	rows, err := server.ListInstalledPackages()
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
	gateway.EnrichDraftModels(cfg, draft)
	return gateway.BuildDraftPlan(cfg, current, draft), nil
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

func (a *nativeComplexPreview) invalidateJSONDraftValidation() {
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
	if err := gateway.PublishPlan(plan); err != nil {
		return nativeGatewayPreview{}, err
	}
	return loadNativeGatewayPreview()
}

func (a *nativeComplexPreview) view(c *ui.Context) {
	ui.Column(c).Fill().Padding(20).Gap(12).Children(func() {
		ui.Text(c, "Complex UI prototypes").FontSize(24).Bold().Role(ui.RoleHeading).Level(1)
		ui.Text(c, "Validation is read-only. Publishing a draft requires explicit confirmation and writes models.json only; config and request history are not modified.").TextColor(c.Theme().TextMuted)
		ui.Segmented(c, &a.tab, "JSON Editor", "Gateway Diff", "Stats & Sessions", "Packages").Label("Prototype section")
		ui.Scroll(c).Fill().Gap(12).Children(func() {
			switch a.tab {
			case 0:
				a.jsonEditor(c)
			case 1:
				a.gatewayDiff(c)
			case 2:
				a.statsAndSessions(c)
			case 3:
				a.packagesSection(c)
			}
		})
	})
}

func (a *nativeComplexPreview) packagesSection(c *ui.Context) {
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
				result, err := server.ImportPiPackages()
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
				if err := server.AddInstalledPackage(spec, true); err != nil {
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
						enabled, err := server.ToggleInstalledPackage(id)
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
			if err := server.DeleteInstalledPackage(id); err != nil {
				return "", err
			}
			return "Removed registration for " + name, nil
		})
	}
}

func (a *nativeComplexPreview) jsonEditor(c *ui.Context) {
	ui.Text(c, "Gateway JSON draft validation").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
	ui.Text(c, "Load starts from generated pi-switch providers only. Validation is read-only; draft edits are not published.").TextColor(c.Theme().TextMuted)
	editor := ui.TextArea(c, &a.jsonDraft).Label("Draft JSON").Disabled(a.gatewayPublishing).Height(260).Fill()
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
	if ui.Button(c, "Load safe Gateway draft").Disabled(a.gatewayPublishing).Clicked() {
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
	if ui.Button(c, "Validate Gateway draft").Disabled(a.gatewayPublishing).Clicked() {
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
	if ui.PrimaryButton(c, "Publish validated Gateway draft").Disabled(a.publishGatewayDraft == nil || a.gatewayPublishing || !a.jsonDraftValidated || a.jsonDraftPending == 0).Clicked() {
		a.jsonPublishConfirm = true
	}
	if ui.AlertDialog(c, &a.jsonPublishConfirm, "Publish Gateway draft?",
		fmt.Sprintf("Write this Gateway draft to %s? Third-party providers are preserved.", gateway.ModelsPath()),
		"Cancel", "Publish") == 1 && a.publishGatewayDraft != nil {
		a.gatewayPublishMessage = ""
		a.publishGatewayDraft(a.jsonDraft)
	}
}

func (a *nativeComplexPreview) gatewayDiff(c *ui.Context) {
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

func nativeGatewaySelections(preview *nativeComplexPreview) []gateway.GatewaySelection {
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

func (a *nativeComplexPreview) statsAndSessions(c *ui.Context) {
	ui.Text(c, "Usage summary · local request history").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
	refreshClicked := ui.Button(c, "Refresh Stats").Disabled(a.refreshStats == nil || a.statsLoading).Clicked()
	if refreshClicked && a.refreshStats != nil {
		a.refreshStats(a.stats.requestPage, a.stats.conversationPage, a.statsFilters)
	}
	if ui.Segmented(c, &a.statsFilters.requestRange, "Today", "24h", "7d", "Custom", "All time").Label("Request range").Disabled(a.refreshStats == nil || a.statsLoading).Changed() && a.refreshStats != nil {
		a.refreshStats(0, a.stats.conversationPage, a.statsFilters)
	}
	if a.statsFilters.requestRange == nativeRangeCustom {
		ui.Row(c).Gap(8).Children(func() {
			fromChanged := ui.DateInput(c, &a.statsFilters.requestFrom).Label("Request from").Changed()
			toChanged := ui.DateInput(c, &a.statsFilters.requestTo).Label("Request to").Changed()
			if (fromChanged || toChanged) && a.refreshStats != nil {
				a.refreshStats(0, a.stats.conversationPage, a.statsFilters)
			}
		})
	}
	if ui.Segmented(c, &a.statsFilters.conversationRange, "Today", "24h", "7d", "Custom dates", "Full history").Label("Conversation range").Disabled(a.refreshStats == nil || a.statsLoading).Changed() && a.refreshStats != nil {
		a.refreshStats(a.stats.requestPage, 0, a.statsFilters)
	}
	if a.statsFilters.conversationRange == nativeRangeCustom {
		ui.Row(c).Gap(8).Children(func() {
			fromChanged := ui.DateInput(c, &a.statsFilters.conversationFrom).Label("Conversation from").Changed()
			toChanged := ui.DateInput(c, &a.statsFilters.conversationTo).Label("Conversation to").Changed()
			if (fromChanged || toChanged) && a.refreshStats != nil {
				a.refreshStats(a.stats.requestPage, 0, a.statsFilters)
			}
		})
	}
	if a.statsLoading {
		ui.Text(c, "Loading request history…").Role(ui.RoleStatus)
	}
	if a.statsError != "" {
		ui.Text(c, a.statsError).Role(ui.RoleStatus)
		if a.statsLoaded {
			ui.Text(c, "Showing the last successful stats snapshot.").TextColor(c.Theme().TextMuted)
		}
	}
	if !a.statsLoaded {
		ui.Text(c, "Refresh to load request history.").TextColor(c.Theme().TextMuted)
		return
	}
	cost := "unknown"
	if a.stats.totalCost != nil {
		cost = fmt.Sprintf("$%.2f", *a.stats.totalCost)
	}
	latency := "-"
	if a.stats.totalRequests > 0 {
		latency = fmt.Sprintf("%d ms", a.stats.avgLatencyMs)
	}
	ui.Text(c, fmt.Sprintf("Requests: %d · successful: %d (%s) · failed: %d · cost: %s · unknown cost: %d", a.stats.totalRequests, a.stats.okRequests, a.stats.successRate, a.stats.failedRequests, cost, a.stats.costUnknown))
	ui.Text(c, fmt.Sprintf("Tokens: input %s · output %s · cached %s · reasoning %s · total %s · cache rate: %s · avg latency: %s",
		nativeTokenDimension(a.stats.totalTokens["input"]), nativeTokenDimension(a.stats.totalTokens["output"]),
		nativeTokenDimension(a.stats.totalTokens["cached"]), nativeTokenDimension(a.stats.totalTokens["reasoning"]),
		nativeTokenDimension(a.stats.totalTokens["total"]), a.stats.cacheHitRate, latency))
	if len(a.stats.providerRows) > 0 {
		ui.Collapsible(c, fmt.Sprintf("Requests by provider (%d)", len(a.stats.providerRows)), &a.providerChartOpen, func() {
			total := 0
			for _, row := range a.stats.providerRows {
				total += row.requests
			}
			if total == 0 {
				ui.Text(c, "No request data.").TextColor(c.Theme().TextMuted)
				return
			}
			a.providerChartList.Key = func(i int) any { return a.stats.providerRows[i].key }
			a.providerChartList.Label = func(i int) string {
				row := a.stats.providerRows[i]
				return fmt.Sprintf("%s · %d%% of requests", row.key, int(float64(row.requests)*100/float64(total)))
			}
			ui.List(c, &a.providerChartList, len(a.stats.providerRows), func(i int) {
				row := a.stats.providerRows[i]
				label := a.providerChartList.Label(i)
				ui.Column(c).Gap(4).Padding(6, 8).Children(func() {
					ui.Text(c, label)
					ui.Progress(c, float64(row.requests)/float64(total)).Label(label).FillWidth()
				})
			}).Label("Provider request share").Height(180)
		})
		ui.Collapsible(c, fmt.Sprintf("By provider (%d)", len(a.stats.providerRows)), &a.providersOpen, func() {
			a.providerList.Key = func(i int) any { return a.stats.providerRows[i].key }
			a.providerList.Label = func(i int) string { return a.stats.providerRows[i].label }
			a.providerList.Selected = &a.providerSelected
			ui.List(c, &a.providerList, len(a.stats.providerRows), func(i int) {
				ui.Text(c, a.stats.providerRows[i].label).Padding(6, 8)
			}).Label("Providers").Height(180)
		})
	}
	if len(a.stats.modelRows) > 0 {
		ui.Collapsible(c, fmt.Sprintf("By model (%d)", len(a.stats.modelRows)), &a.modelsOpen, func() {
			a.modelList.Key = func(i int) any { return a.stats.modelRows[i].key }
			a.modelList.Label = func(i int) string { return a.stats.modelRows[i].label }
			a.modelList.Selected = &a.modelSelected
			ui.List(c, &a.modelList, len(a.stats.modelRows), func(i int) {
				ui.Text(c, a.stats.modelRows[i].label).Padding(6, 8)
			}).Label("Models").Height(180)
		})
	}
	lastPage := max(0, (a.stats.requestTotal-1)/10)
	ui.Row(c).Gap(8).Children(func() {
		previousClicked := ui.Button(c, "Previous page").Disabled(a.refreshStats == nil || a.statsLoading || a.stats.requestPage == 0).Clicked()
		if previousClicked && a.refreshStats != nil {
			a.refreshStats(a.stats.requestPage-1, a.stats.conversationPage, a.statsFilters)
		}
		nextClicked := ui.Button(c, "Next page").Disabled(a.refreshStats == nil || a.statsLoading || a.stats.requestPage >= lastPage).Clicked()
		if nextClicked && a.refreshStats != nil {
			a.refreshStats(a.stats.requestPage+1, a.stats.conversationPage, a.statsFilters)
		}
	})
	ui.Text(c, fmt.Sprintf("Recent requests · page %d of %d", a.stats.requestPage+1, lastPage+1))
	if len(a.stats.rows) == 0 {
		ui.Text(c, "No request data.").TextColor(c.Theme().TextMuted)
	} else {
		if a.selectedUsage < 0 || a.selectedUsage >= len(a.stats.rows) {
			a.selectedUsage = 0
		}
		a.usageList.Key = func(i int) any { return a.stats.rows[i].name }
		a.usageList.Label = func(i int) string { return nativeRecentRequestText(a.stats.rows[i]) }
		a.usageList.Selected = &a.selectedUsage
		ui.List(c, &a.usageList, len(a.stats.rows), func(i int) {
			ui.Text(c, nativeRecentRequestText(a.stats.rows[i])).Padding(6, 8)
		}).Label("Recent requests").Height(180)
	}
	ui.Text(c, "Conversations · recent request matches and full history").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
	if a.stats.conversationSource == "off" {
		ui.Text(c, "Conversation attribution is disabled in settings.").TextColor(c.Theme().TextMuted)
	} else if len(a.stats.conversations) == 0 {
		ui.Text(c, "No attributed conversations.").TextColor(c.Theme().TextMuted)
	} else {
		lastConversationPage := max(0, (a.stats.conversationTotal-1)/10)
		ui.Row(c).Gap(8).Children(func() {
			previousClicked := ui.Button(c, "Previous conversation page").Disabled(a.refreshStats == nil || a.statsLoading || a.stats.conversationPage == 0).Clicked()
			if previousClicked && a.refreshStats != nil {
				a.refreshStats(a.stats.requestPage, a.stats.conversationPage-1, a.statsFilters)
			}
			nextClicked := ui.Button(c, "Next conversation page").Disabled(a.refreshStats == nil || a.statsLoading || a.stats.conversationPage >= lastConversationPage).Clicked()
			if nextClicked && a.refreshStats != nil {
				a.refreshStats(a.stats.requestPage, a.stats.conversationPage+1, a.statsFilters)
			}
		})
		ui.Text(c, fmt.Sprintf("Conversation page %d of %d · %d conversations", a.stats.conversationPage+1, lastConversationPage+1, a.stats.conversationTotal))
		roots := make([]string, 0, len(a.stats.conversations))
		labels := make(map[string]string, len(a.stats.conversations))
		children := make(map[string][]string, len(a.stats.conversations))
		for _, conversation := range a.stats.conversations {
			roots = append(roots, conversation.id)
			labels[conversation.id] = conversation.label
			children[conversation.id] = conversation.requests
		}
		a.sessionOutline.List.Label = func(i int) string {
			item := a.sessionOutline.Item(i)
			if label, ok := labels[item]; ok {
				return label
			}
			return item
		}
		ui.Outline(c, &a.sessionOutline, roots, func(item string) []string {
			return children[item]
		}, func(item string) {
			if label, ok := labels[item]; ok {
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, label).Padding(4, 8)
					if ui.Button(c, "View requests · "+item).Disabled(a.refreshConversationRequests == nil || a.conversationRequestsLoading).Clicked() && a.refreshConversationRequests != nil {
						a.refreshConversationRequests(item, 0)
					}
				})
				return
			}
			ui.Text(c, item).Padding(4, 8)
		}).Label("Conversations").Height(180)
		if a.conversationRequests.id != "" {
			ui.Text(c, "Full request history · "+a.conversationRequests.id).Role(ui.RoleHeading).Level(3)
			if a.conversationRequestsLoading {
				ui.Text(c, "Loading conversation requests…").Role(ui.RoleStatus)
			} else if a.conversationRequestsError != "" {
				ui.Text(c, a.conversationRequestsError).Role(ui.RoleStatus)
			} else {
				lastPage := max(0, (a.conversationRequests.total-1)/10)
				ui.Row(c).Gap(8).Children(func() {
					previousClicked := ui.Button(c, "Previous conversation request page").Disabled(a.conversationRequestsLoading || a.conversationRequests.page == 0).Clicked()
					if previousClicked && a.refreshConversationRequests != nil {
						a.refreshConversationRequests(a.conversationRequests.id, a.conversationRequests.page-1)
					}
					nextClicked := ui.Button(c, "Next conversation request page").Disabled(a.refreshConversationRequests == nil || a.conversationRequestsLoading || a.conversationRequests.page >= lastPage).Clicked()
					if nextClicked && a.refreshConversationRequests != nil {
						a.refreshConversationRequests(a.conversationRequests.id, a.conversationRequests.page+1)
					}
				})
				ui.Text(c, fmt.Sprintf("Conversation request page %d of %d · %d requests", a.conversationRequests.page+1, lastPage+1, a.conversationRequests.total))
				if len(a.conversationRequests.rows) == 0 {
					ui.Text(c, "No requests in this conversation.").TextColor(c.Theme().TextMuted)
				} else {
					for _, row := range a.conversationRequests.rows {
						ui.Text(c, row).Padding(4, 8)
					}
				}
			}
		}
	}
}

func usageRowText(row nativeUsageRow) string {
	tokens, cost := "unknown", "unknown"
	if row.tokens != nil {
		tokens = fmt.Sprint(*row.tokens)
	}
	if row.cost != nil {
		cost = fmt.Sprintf("$%.2f", *row.cost)
	}
	return fmt.Sprintf("%s · tokens: %s · cost: %s", row.name, tokens, cost)
}

func nativeRecentRequestText(row nativeUsageRow) string {
	cacheRate := row.cacheRate
	if cacheRate == "" {
		cacheRate = "-"
	}
	return fmt.Sprintf("%s · %s · %s · input: %s · output: %s · cached: %s · reasoning: %s · cache rate: %s · total: %s · cost: %s",
		row.timestamp, row.name, row.status, nativeTokenText(row.promptTokens), nativeTokenText(row.completionTokens),
		nativeTokenText(row.cachedTokens), nativeTokenText(row.reasoningTokens), cacheRate, nativeTokenText(row.tokens), nativeCostText(row.cost))
}

func nativeTokenDimension(value int64) string {
	if value == 0 {
		return "-"
	}
	return fmt.Sprint(value)
}

func nativeCostText(value *float64) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprintf("$%.2f", *value)
}
