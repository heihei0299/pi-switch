package analytics

import (
	"testing"

	"github.com/heihei0299/pi-switch/internal/domain/conversation"
)

func TestUnknownUsagePreservedInDetailAndCacheDenominator(t *testing.T) {
	rows := factMemory{
		{ID: 1, TS: ptr("2026-09-10T12:00:00Z"), Provider: ptr("p"), Model: ptr("m"), Success: ptr(true), PromptTokens: ptr(int64(100)), CompletionTokens: ptr(int64(5)), ConversationID: ptr("c")},
		{ID: 2, TS: ptr("2026-09-10T12:01:00Z"), Provider: ptr("p"), Model: ptr("m"), Success: ptr(true), PromptTokens: ptr(int64(100)), CompletionTokens: ptr(int64(5)), CachedTokens: ptr(int64(50)), ReasoningTokens: ptr(int64(0)), ConversationID: ptr("c")},
	}
	service := NewService(rows, conversation.SourceProxy, nil)
	requests, _, err := service.ConversationRequests("c", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || requests[1].CachedTokens != nil || requests[1].ReasoningTokens != nil || requests[1].CacheRate != "-" {
		t.Fatalf("detail fabricated usage: %+v", requests)
	}
	got, err := service.Stats(nil, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if got.CacheHitRate != "50.0%" || got.ByProvider["p"].CacheRate != "50.0%" || got.ByModel["m"].CacheRate != "50.0%" || got.ByConversation[0].CacheRate != "50.0%" {
		t.Fatalf("unknown cache samples diluted rate: %+v", got)
	}
}

func TestPartialOrFailedUsageHasUnknownRequestCacheRate(t *testing.T) {
	service := NewService(factMemory{
		{ID: 1, TS: ptr("2026-09-10T12:00:00Z"), Provider: ptr("p"), Model: ptr("m"), Success: ptr(false), PromptTokens: ptr(int64(100)), CompletionTokens: ptr(int64(5)), CachedTokens: ptr(int64(50)), ConversationID: ptr("c")},
		{ID: 2, TS: ptr("2026-09-10T12:01:00Z"), Provider: ptr("p"), Model: ptr("m"), Success: ptr(true), PromptTokens: ptr(int64(100)), CachedTokens: ptr(int64(50)), ConversationID: ptr("c")},
	}, conversation.SourceProxy, nil)
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
