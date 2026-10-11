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
