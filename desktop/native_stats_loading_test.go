package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestNativeStatsCustomDateInputsStayDisabledWhileLoading(t *testing.T) {
	for _, field := range []string{"Request from", "Request to", "Conversation from", "Conversation to"} {
		t.Run(field, func(t *testing.T) {
			preview := newNativeComplexPreview()
			preview.tab = 2
			preview.statsLoading = true
			preview.statsFilters.requestRange = nativeRangeCustom
			preview.statsFilters.conversationRange = nativeRangeCustom
			preview.statsFilters.requestFrom = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
			preview.statsFilters.requestTo = preview.statsFilters.requestFrom
			preview.statsFilters.conversationFrom = preview.statsFilters.requestFrom
			preview.statsFilters.conversationTo = preview.statsFilters.requestFrom
			var refreshes int
			preview.refreshStats = func(int, int, nativeStatsFilters) { refreshes++ }
			tester := ui.NewTester(preview.view, 900, 1000)

			if err := tester.Click(field); err != nil {
				t.Fatal(err)
			}
			if err := tester.Click("January 2, 2026"); err == nil {
				t.Fatalf("%s changed while the previous stats query was loading", field)
			}
			if refreshes != 0 || !preview.statsFilters.requestFrom.Equal(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)) {
				t.Fatalf("loading stats accepted a date change: refreshes=%d filters=%+v", refreshes, preview.statsFilters)
			}
		})
	}
}

func TestNativeStatsOpenDateChangeRestartsTheCurrentRequest(t *testing.T) {
	preview := newNativeComplexPreview()
	preview.tab = 2
	preview.stats.requestPage = 4
	preview.stats.conversationPage = 3
	preview.statsFilters.requestRange = nativeRangeCustom
	preview.statsFilters.requestFrom = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	loadedFilters := preview.statsFilters
	var refreshes, requestPage, conversationPage int
	preview.refreshStats = func(request, conversation int, _ nativeStatsFilters) {
		refreshes++
		requestPage, conversationPage = request, conversation
	}
	tester := ui.NewTester(preview.view, 900, 1000)
	if err := tester.Click("Request from"); err != nil {
		t.Fatal(err)
	}
	if !tester.HasText("January 2026") {
		t.Fatal("date calendar did not open")
	}

	preview.statsLoading = true
	tester.Frame()
	if err := tester.Click("January 2, 2026"); err != nil {
		t.Fatalf("open calendar did not accept the current date selection: %v", err)
	}
	if refreshes != 1 || requestPage != 0 || conversationPage != 3 || !preview.statsFilters.requestFrom.Equal(time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("current date selection = calls %d, pages %d/%d, filters %+v", refreshes, requestPage, conversationPage, preview.statsFilters)
	}
	requestPage, conversationPage, changed := nativeStatsPagesForLatestFilters(4, 3, loadedFilters, preview.statsFilters)
	if !changed || requestPage != 0 || conversationPage != 3 {
		t.Fatalf("request-date retry = changed %v, pages %d/%d", changed, requestPage, conversationPage)
	}
}

func TestNativeStatsPagesForLatestFiltersResetsOnlyChangedRange(t *testing.T) {
	loaded := newNativeStatsFilters(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
	selected := loaded
	selected.conversationTo = selected.conversationTo.AddDate(0, 0, 1)
	requestPage, conversationPage, changed := nativeStatsPagesForLatestFilters(4, 3, loaded, selected)
	if !changed || requestPage != 4 || conversationPage != 0 {
		t.Fatalf("conversation-date retry = changed %v, pages %d/%d", changed, requestPage, conversationPage)
	}
	requestPage, conversationPage, changed = nativeStatsPagesForLatestFilters(4, 3, loaded, loaded)
	if changed || requestPage != 4 || conversationPage != 3 {
		t.Fatalf("unchanged filters requested a retry: changed %v, pages %d/%d", changed, requestPage, conversationPage)
	}
}
