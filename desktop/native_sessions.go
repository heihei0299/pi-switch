package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/egoist/mygo/ui"
	statsservice "github.com/heihei0299/pi-switch/internal/stats"
	"github.com/heihei0299/pi-switch/internal/store"
)

type nativeSessionsPage struct {
	conversationPage   int
	conversationTotal  int
	conversationSource string
	conversations      []nativeConversation
}

type nativeSessionsState struct {
	sessions                    nativeSessionsPage
	sessionsFilters             nativeSessionsFilters
	conversationRequests        nativeConversationPage
	conversationRequestsLoading bool
	conversationRequestsError   string
	refreshConversationRequests func(string, int)
	sessionOutline              ui.OutlineState[string]
}

func (a *nativeSessionsState) filtersSection(c *ui.Context, stats *nativeStatsState) {
	if ui.Segmented(c, &a.sessionsFilters.conversationRange, "Today", "24h", "7d", "Custom dates", "Full history").Label("Conversation range").Disabled(stats.refreshStats == nil || stats.statsLoading).Changed() && stats.refreshStats != nil {
		stats.refreshStats(stats.stats.requestPage, 0, stats.statsFilters, a.sessionsFilters)
	}
	if a.sessionsFilters.conversationRange == nativeRangeCustom {
		ui.Row(c).Gap(8).Children(func() {
			fromChanged := ui.DateInput(c, &a.sessionsFilters.conversationFrom).Label("Conversation from").Disabled(stats.statsLoading).Changed()
			toChanged := ui.DateInput(c, &a.sessionsFilters.conversationTo).Label("Conversation to").Disabled(stats.statsLoading).Changed()
			if (fromChanged || toChanged) && stats.refreshStats != nil {
				stats.refreshStats(stats.stats.requestPage, 0, stats.statsFilters, a.sessionsFilters)
			}
		})
	}
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

func loadNativeConversationPage(id string, page, limit int) (nativeConversationPage, error) {
	if _, err := os.Stat(store.DBPath()); errors.Is(err, os.ErrNotExist) {
		return nativeConversationPage{id: id, page: page}, nil
	} else if err != nil {
		return nativeConversationPage{}, fmt.Errorf("inspect request history: %w", err)
	}
	service, err := statsservice.OpenManagementService()
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

func (a *nativeSessionsState) section(c *ui.Context, stats *nativeStatsState) {
	ui.Text(c, "Conversations · recent request matches and full history").FontSize(18).Bold().Role(ui.RoleHeading).Level(2)
	if a.sessions.conversationSource == "off" {
		ui.Text(c, "Conversation attribution is disabled in settings.").TextColor(c.Theme().TextMuted)
	} else if len(a.sessions.conversations) == 0 {
		ui.Text(c, "No attributed conversations.").TextColor(c.Theme().TextMuted)
	} else {
		lastConversationPage := max(0, (a.sessions.conversationTotal-1)/10)
		ui.Row(c).Gap(8).Children(func() {
			previousClicked := ui.Button(c, "Previous conversation page").Disabled(stats.refreshStats == nil || stats.statsLoading || a.sessions.conversationPage == 0).Clicked()
			if previousClicked && stats.refreshStats != nil {
				stats.refreshStats(stats.stats.requestPage, a.sessions.conversationPage-1, stats.statsFilters, a.sessionsFilters)
			}
			nextClicked := ui.Button(c, "Next conversation page").Disabled(stats.refreshStats == nil || stats.statsLoading || a.sessions.conversationPage >= lastConversationPage).Clicked()
			if nextClicked && stats.refreshStats != nil {
				stats.refreshStats(stats.stats.requestPage, a.sessions.conversationPage+1, stats.statsFilters, a.sessionsFilters)
			}
		})
		ui.Text(c, fmt.Sprintf("Conversation page %d of %d · %d conversations", a.sessions.conversationPage+1, lastConversationPage+1, a.sessions.conversationTotal))
		roots := make([]string, 0, len(a.sessions.conversations))
		labels := make(map[string]string, len(a.sessions.conversations))
		children := make(map[string][]string, len(a.sessions.conversations))
		for _, conversation := range a.sessions.conversations {
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
