package stats

import (
	"testing"

	"github.com/heihei0299/pi-switch/internal/conversation"
)

func TestUnknownUsagePreservedInDetailAndCacheDenominator(t *testing.T) {
	db := openStatsDB(t)
	insertFact(t, db, "2026-09-10T12:00:00Z", "p", "m", 1, int64(100), int64(5), nil, nil, nil, "c", nil, nil)
	service := Service{DB: db, Source: conversation.SourceProxy}
	rows, _, err := service.ConversationRequests("c", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].CachedTokens != nil || rows[0].ReasoningTokens != nil || rows[0].CacheRate != "-" {
		t.Fatalf("detail fabricated usage: %+v", rows)
	}
	insertFact(t, db, "2026-09-10T12:01:00Z", "p", "m", 1, int64(100), int64(5), int64(50), int64(0), nil, "c", nil, nil)
	got, err := service.Stats(nil, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if got.CacheHitRate != "50.0%" || got.ByProvider["p"].CacheRate != "50.0%" || got.ByModel["m"].CacheRate != "50.0%" || got.ByConversation[0].CacheRate != "50.0%" {
		t.Fatalf("unknown cache samples diluted rate: %+v", got)
	}
}

func TestPartialOrFailedUsageHasUnknownRequestCacheRate(t *testing.T) {
	db := openStatsDB(t)
	insertFact(t, db, "2026-09-10T12:00:00Z", "p", "m", 0, int64(100), int64(5), int64(50), nil, nil, "c", nil, nil)
	insertFact(t, db, "2026-09-10T12:01:00Z", "p", "m", 1, int64(100), nil, int64(50), nil, nil, "c", nil, nil)
	service := Service{DB: db, Source: conversation.SourceProxy}
	got, err := service.Stats(nil, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range got.RecentRequests {
		if row.CacheRate != "-" || row.PromptTokens == nil || row.CachedTokens == nil {
			t.Errorf("partial/failed request rate=%s, facts=%+v", row.CacheRate, row)
		}
	}
	rows, _, err := service.ConversationRequests("c", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.CacheRate != "-" || row.PromptTokens == nil || row.CachedTokens == nil {
			t.Errorf("partial/failed detail rate=%s, facts=%+v", row.CacheRate, row)
		}
	}
}
