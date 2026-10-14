package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/heihei0299/pi-switch/internal/bootstrap"
	"github.com/heihei0299/pi-switch/internal/store"
	"github.com/heihei0299/pi-switch/internal/usecase/analytics"
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
type nativeStatsPage struct {
	requestPage    int
	totalRequests  int
	requestTotal   int
	okRequests     int
	failedRequests int
	successRate    string
	avgLatencyMs   int64
	totalTokens    map[string]int64
	cacheHitRate   string
	totalCost      *float64
	costUnknown    int64
	providerRows   []nativeStatsRow
	modelRows      []nativeStatsRow
	rows           []nativeUsageRow
}

type nativeStatsSnapshot struct {
	nativeStatsPage
	nativeSessionsPage
}

type nativeStatsState struct {
	stats             nativeStatsPage
	statsLoaded       bool
	statsLoading      bool
	statsError        string
	statsExportStatus string
	statsExporting    bool
	statsFilters      nativeStatsFilters
	statsRefreshTier  int
	statsRefreshAt    time.Time
	refreshStats      func(int, int, nativeStatsFilters, nativeSessionsFilters)
	exportStats       func(string)
	usageList         ui.ListState
	providerList      ui.ListState
	providerChartList ui.ListState
	modelList         ui.ListState
	providerSelected  int
	modelSelected     int
	providerChartOpen bool
	providersOpen     bool
	modelsOpen        bool
	selectedUsage     int
}
type nativeStatsRow struct {
	key      string
	label    string
	requests int
}
type nativeStatsFilters struct {
	requestRange int
	requestFrom  time.Time
	requestTo    time.Time
}

type nativeSessionsFilters struct {
	conversationRange int
	conversationFrom  time.Time
	conversationTo    time.Time
}

func nativeStatsPagesForLatestFilters(requestPage, conversationPage int, loadedStats nativeStatsFilters, loadedSessions nativeSessionsFilters, selectedStats nativeStatsFilters, selectedSessions nativeSessionsFilters) (int, int, bool) {
	requestChanged := loadedStats.requestRange != selectedStats.requestRange || !loadedStats.requestFrom.Equal(selectedStats.requestFrom) || !loadedStats.requestTo.Equal(selectedStats.requestTo)
	conversationChanged := loadedSessions.conversationRange != selectedSessions.conversationRange || !loadedSessions.conversationFrom.Equal(selectedSessions.conversationFrom) || !loadedSessions.conversationTo.Equal(selectedSessions.conversationTo)
	if requestChanged {
		requestPage = 0
	}
	if conversationChanged {
		conversationPage = 0
	}
	return requestPage, conversationPage, requestChanged || conversationChanged
}
func nativeStatsRefreshInterval(tier int) time.Duration {
	switch tier {
	case 1:
		return 5 * time.Second
	case 2:
		return 30 * time.Second
	case 3:
		return 5 * time.Minute
	default:
		return 0
	}
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
		requestFrom: today,
		requestTo:   today,
	}
}

func newNativeSessionsFilters(now time.Time) nativeSessionsFilters {
	today := nativeCalendarDay(now)
	return nativeSessionsFilters{
		conversationFrom: today,
		conversationTo:   today,
	}
}
func nativeStatsWindow(rangeChoice int, fromDate, toDate, now time.Time) (*analytics.Window, error) {
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
	return &analytics.Window{From: from.UnixMilli(), To: to.UnixMilli()}, nil
}
func loadNativeStats(requestPage, conversationPage, limit int, statsFilters nativeStatsFilters, sessionsFilters nativeSessionsFilters) (nativeStatsSnapshot, error) {
	now := time.Now()
	requestWindow, err := nativeStatsWindow(statsFilters.requestRange, statsFilters.requestFrom, statsFilters.requestTo, now)
	if err != nil {
		return nativeStatsSnapshot{}, err
	}
	conversationWindow, err := nativeStatsWindow(sessionsFilters.conversationRange, sessionsFilters.conversationFrom, sessionsFilters.conversationTo, now)
	if err != nil {
		return nativeStatsSnapshot{}, err
	}
	if _, err := os.Stat(store.DBPath()); errors.Is(err, os.ErrNotExist) {
		return nativeStatsSnapshot{
			nativeStatsPage:    nativeStatsPage{requestPage: requestPage, successRate: "0%", cacheHitRate: "-"},
			nativeSessionsPage: nativeSessionsPage{conversationPage: conversationPage},
		}, nil
	} else if err != nil {
		return nativeStatsSnapshot{}, fmt.Errorf("inspect request history: %w", err)
	}
	service, err := bootstrap.OpenManagementAnalyticsService()
	if err != nil {
		return nativeStatsSnapshot{}, err
	}
	result, conversations, conversationTotal, err := service.StatsAndConversations(requestWindow, conversationWindow, requestPage, conversationPage, limit)
	if err != nil {
		return nativeStatsSnapshot{}, err
	}
	loaded := nativeStatsSnapshot{
		nativeStatsPage: nativeStatsPage{
			requestPage:    requestPage,
			totalRequests:  result.TotalRequests,
			requestTotal:   result.RecentRequestTotal,
			okRequests:     result.OKRequests,
			failedRequests: result.FailedRequests,
			successRate:    result.SuccessRate,
			avgLatencyMs:   result.AvgLatencyMs,
			totalTokens:    result.TotalTokens,
			cacheHitRate:   result.CacheHitRate,
			totalCost:      result.TotalCost,
			costUnknown:    result.CostUnknown,
			rows:           make([]nativeUsageRow, 0, len(result.RecentRequests)),
		},
		nativeSessionsPage: nativeSessionsPage{
			conversationPage:   conversationPage,
			conversationTotal:  conversationTotal,
			conversationSource: string(service.Source),
		},
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
func writeNativeStatsExport(path, format string, service analytics.Service) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".pi-switch-stats-export-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := service.ExportRequests(temp, format); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
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

func (a *nativeStatsState) section(c *ui.Context, sessions *nativeSessionsState) bool {
	ui.Text(c, "Usage summary · local request history").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
	refreshClicked := ui.Button(c, "Refresh Stats").Disabled(a.refreshStats == nil || a.statsLoading).Clicked()
	if refreshClicked && a.refreshStats != nil {
		a.refreshStats(a.stats.requestPage, sessions.sessions.conversationPage, a.statsFilters, sessions.sessionsFilters)
	}
	if ui.Segmented(c, &a.statsRefreshTier, "Off", "5s", "30s", "5min").Label("Auto-refresh").Disabled(a.refreshStats == nil).Changed() {
		a.statsRefreshAt = time.Time{}
	}
	if interval := nativeStatsRefreshInterval(a.statsRefreshTier); interval > 0 && a.refreshStats != nil {
		now := c.Now()
		if a.statsRefreshAt.IsZero() {
			a.statsRefreshAt = now.Add(interval)
		}
		if !now.Before(a.statsRefreshAt) {
			if !a.statsLoading {
				a.refreshStats(a.stats.requestPage, sessions.sessions.conversationPage, a.statsFilters, sessions.sessionsFilters)
			}
			a.statsRefreshAt = now.Add(interval)
		}
		c.After(a.statsRefreshAt.Sub(now))
	} else {
		a.statsRefreshAt = time.Time{}
	}
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, "Export JSON").Disabled(a.exportStats == nil || a.statsExporting).Clicked() && a.exportStats != nil {
			a.exportStats("json")
		}
		if ui.Button(c, "Export CSV").Disabled(a.exportStats == nil || a.statsExporting).Clicked() && a.exportStats != nil {
			a.exportStats("csv")
		}
	})
	ui.Text(c, "Exports the full request history; error text and upstream URLs may contain sensitive data.").TextColor(c.Theme().TextMuted)
	if a.statsExportStatus != "" {
		ui.Text(c, a.statsExportStatus).Role(ui.RoleStatus)
	}
	if ui.Segmented(c, &a.statsFilters.requestRange, "Today", "24h", "7d", "Custom", "All time").Label("Request range").Disabled(a.refreshStats == nil || a.statsLoading).Changed() && a.refreshStats != nil {
		a.refreshStats(0, sessions.sessions.conversationPage, a.statsFilters, sessions.sessionsFilters)
	}
	if a.statsFilters.requestRange == nativeRangeCustom {
		ui.Row(c).Gap(8).Children(func() {
			fromChanged := ui.DateInput(c, &a.statsFilters.requestFrom).Label("Request from").Disabled(a.statsLoading).Changed()
			toChanged := ui.DateInput(c, &a.statsFilters.requestTo).Label("Request to").Disabled(a.statsLoading).Changed()
			if (fromChanged || toChanged) && a.refreshStats != nil {
				a.refreshStats(0, sessions.sessions.conversationPage, a.statsFilters, sessions.sessionsFilters)
			}
		})
	}
	sessions.filtersSection(c, a)
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
		return false
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
			a.refreshStats(a.stats.requestPage-1, sessions.sessions.conversationPage, a.statsFilters, sessions.sessionsFilters)
		}
		nextClicked := ui.Button(c, "Next page").Disabled(a.refreshStats == nil || a.statsLoading || a.stats.requestPage >= lastPage).Clicked()
		if nextClicked && a.refreshStats != nil {
			a.refreshStats(a.stats.requestPage+1, sessions.sessions.conversationPage, a.statsFilters, sessions.sessionsFilters)
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
	return true
}

func (p *nativeStatsState) refresh(shell *desktopShell, sessions *nativeSessionsState, requestPage, conversationPage int, statsFilters nativeStatsFilters, sessionsFilters nativeSessionsFilters) {
	if shell.window == nil || p.statsLoading || shell.exiting {
		return
	}
	preview := p
	window := shell.window
	ctx, token, ok := shell.tasks.begin("stats")
	if !ok {
		return
	}
	preview.statsLoading = true
	preview.statsError = ""
	window.Invalidate()
	go func() {
		loaded, err := loadNativeStats(requestPage, conversationPage, 10, statsFilters, sessionsFilters)
		if ctx.Err() != nil {
			return
		}
		window.Update(func() {
			if window.IsDestroyed() || !shell.tasks.accept("stats", token) {
				shell.tasks.finish("stats", token)
				return
			}
			nextRequestPage, nextConversationPage, filtersChanged := nativeStatsPagesForLatestFilters(requestPage, conversationPage, statsFilters, sessionsFilters, preview.statsFilters, sessions.sessionsFilters)
			if filtersChanged {
				preview.statsLoading = false
				shell.tasks.finish("stats", token)
				p.refresh(shell, sessions, nextRequestPage, nextConversationPage, preview.statsFilters, sessions.sessionsFilters)
				return
			}
			defer shell.tasks.finish("stats", token)
			preview.statsLoading = false
			if err != nil {
				preview.statsError = err.Error()
				return
			}
			preview.stats = loaded.nativeStatsPage
			sessions.sessions = loaded.nativeSessionsPage
			preview.statsLoaded = true
			preview.statsError = ""
			preview.selectedUsage = -1
		})
	}()
}

func (p *nativeStatsState) export(shell *desktopShell, format string) {
	if shell.window == nil || p.statsExporting || shell.exiting || (format != "json" && format != "csv") {
		return
	}
	preview := p
	window := shell.window
	ctx, token, ok := shell.tasks.begin("stats-export")
	if !ok {
		return
	}
	preview.statsExporting = true
	preview.statsExportStatus = "Preparing request history export…"
	window.Invalidate()
	go func() {
		var status string
		if _, err := os.Stat(store.DBPath()); errors.Is(err, os.ErrNotExist) {
			status = "No request history to export."
		} else if err != nil {
			status = "Export failed: inspect request history: " + err.Error()
		} else {
			db, err := store.GetDB()
			if err != nil {
				status = "Export failed: " + err.Error()
			} else {
				service := bootstrap.NewSummaryAnalyticsService(db)
				filter := mygo.FileFilter{Name: "JSON", Extensions: []string{"json"}}
				if format == "csv" {
					filter = mygo.FileFilter{Name: "CSV", Extensions: []string{"csv"}}
				}
				path, saveErr := mygo.Dialog.Save(mygo.SaveDialogOptions{
					Parent: window, Title: "Export request history", DefaultPath: "pi-switch-logs." + format,
					Filters: []mygo.FileFilter{filter},
				})
				if saveErr != nil {
					status = "Export failed: " + saveErr.Error()
				} else if path == "" {
					status = "Export canceled."
				} else if ctx.Err() == nil {
					if err := writeNativeStatsExport(path, format, service); err != nil {
						status = "Export failed: " + err.Error()
					} else {
						status = "Exported request history to " + filepath.Base(path)
					}
				}
			}
		}
		if ctx.Err() != nil {
			return
		}
		window.Update(func() {
			if window.IsDestroyed() || !shell.tasks.accept("stats-export", token) {
				shell.tasks.finish("stats-export", token)
				return
			}
			defer shell.tasks.finish("stats-export", token)
			preview.statsExporting = false
			preview.statsExportStatus = status
		})
	}()
}
