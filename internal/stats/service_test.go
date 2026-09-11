package stats

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/heihei0299/pi-switch/internal/conversation"
	"github.com/heihei0299/pi-switch/internal/store"
)

func openStatsDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "requests.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insertFact(t *testing.T, db *sql.DB, ts, provider, model string, success any, prompt, completion, cached, reasoning any, cost any, convID, convName any, latency any) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO requests(ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id,conversation_name,latency_ms)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, ts, provider, model, success, prompt, completion, cached, reasoning, cost, convID, convName, latency)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStatsServiceUsesSQLWindowAndPreservesNullableFacts(t *testing.T) {
	db := openStatsDB(t)
	center := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	insertFact(t, db, center.Format(time.RFC3339), "p", "m", 1, int64(10), int64(5), int64(2), nil, 0.25, "c1", "named", int64(12))
	insertFact(t, db, center.Add(-48*time.Hour).Format(time.RFC3339), "old", "old-model", 1, int64(99), int64(1), int64(0), nil, 9.0, "old", nil, int64(99))
	insertFact(t, db, center.Add(time.Minute).Format(time.RFC3339), "failed", "m", 0, nil, int64(3), nil, nil, 3.0, "c2", nil, nil)

	service := Service{DB: db, Source: conversation.SourceProxy}
	response, err := service.Stats(&Window{From: center.Add(-time.Hour).UnixMilli(), To: center.Add(time.Hour).UnixMilli()}, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if response.TotalRequests != 2 || response.OKRequests != 1 || response.FailedRequests != 1 {
		t.Fatalf("unexpected totals: %+v", response)
	}
	if response.TotalCost == nil || *response.TotalCost != 0.25 || response.CostUnknown != 0 {
		t.Fatalf("unexpected cost aggregation: total=%v unknown=%d", response.TotalCost, response.CostUnknown)
	}
	if len(response.RecentRequests) != 2 || response.RecentRequests[0].PromptTokens != nil {
		t.Fatalf("nullable request facts were fabricated: %+v", response.RecentRequests)
	}
	if len(response.ByConversation) != 2 || response.ByConversation[0].ConversationID != "c2" {
		t.Fatalf("unexpected conversations: %+v", response.ByConversation)
	}
}

func TestStatsServiceKeepsMillisecondRightOpenBoundary(t *testing.T) {
	db := openStatsDB(t)
	center := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	recorded := center.Add(250 * time.Millisecond)
	insertFact(t, db, recorded.Format(time.RFC3339Nano), "p", "m", 1, int64(10), int64(5), int64(0), int64(0), nil, nil, nil, nil)
	service := Service{DB: db, Source: conversation.SourceProxy}

	inside, err := service.Stats(&Window{From: center.UnixMilli(), To: center.Add(500 * time.Millisecond).UnixMilli()}, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if inside.TotalRequests != 1 {
		t.Fatalf("subsecond request inside window = %d, want 1", inside.TotalRequests)
	}

	atRightEdge, err := service.Stats(&Window{From: center.UnixMilli(), To: recorded.UnixMilli()}, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if atRightEdge.TotalRequests != 0 {
		t.Fatalf("request exactly at right edge = %d, want 0", atRightEdge.TotalRequests)
	}
}

func TestStatsServiceSharesConversationAttributionAcrossEndpoints(t *testing.T) {
	db := openStatsDB(t)
	ts := "2026-09-10T12:00:00Z"
	insertFact(t, db, ts, "p", "m", 1, int64(10), int64(5), int64(0), int64(0), nil, nil, nil, nil)
	service := Service{DB: db, Source: conversation.SourceProxy}
	conversations, total, err := service.Conversations(nil, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(conversations) != 1 || conversations[0].ConversationID != conversation.UnlabeledID {
		t.Fatalf("unexpected conversation endpoint: total=%d values=%+v", total, conversations)
	}
	requests, requestTotal, err := service.ConversationRequests(conversation.UnlabeledID, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if requestTotal != 1 || len(requests) != 1 || requests[0].ConversationID == nil || *requests[0].ConversationID != conversation.UnlabeledID {
		t.Fatalf("conversation detail did not use same matcher: total=%d values=%+v", requestTotal, requests)
	}
}

func TestStatsServicePaginationIsDeterministic(t *testing.T) {
	db := openStatsDB(t)
	base := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	insertFact(t, db, base.Add(2*time.Minute).Format(time.RFC3339), "p", "m", 1, int64(1), int64(1), int64(0), int64(0), nil, "c2", nil, nil)
	insertFact(t, db, base.Add(1*time.Minute).Format(time.RFC3339), "p", "m", 1, int64(1), int64(1), int64(0), int64(0), nil, "c1", nil, nil)
	service := Service{DB: db, Source: conversation.SourceProxy}
	first, err := service.Stats(nil, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Stats(nil, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.RecentRequests) != 1 || len(second.RecentRequests) != 1 || first.RecentRequests[0].TS == nil || second.RecentRequests[0].TS == nil {
		t.Fatalf("unexpected pages: first=%+v second=%+v", first.RecentRequests, second.RecentRequests)
	}
	// The database id is the stable ingestion order used by the existing API.
	if *first.RecentRequests[0].TS != base.Add(time.Minute).Format(time.RFC3339) || *second.RecentRequests[0].TS != base.Add(2*time.Minute).Format(time.RFC3339) {
		t.Fatalf("pages are not deterministic by request id: first=%s second=%s", *first.RecentRequests[0].TS, *second.RecentRequests[0].TS)
	}
}

// ARCH-06: the TUI status line reads Summary from this package instead of
// running its own SQL. It uses the same countable() rule as Stats: successful
// requests count, but only ones with both prompt and completion usage contribute
// tokens/cost.
func TestSummaryAggregatesSuccessfulRequests(t *testing.T) {
	db := openStatsDB(t)
	center := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	insertFact(t, db, center.Format(time.RFC3339), "p", "m", 1, int64(10), int64(5), int64(2), nil, 0.25, "c1", nil, nil)
	insertFact(t, db, center.Add(time.Minute).Format(time.RFC3339), "p", "m", 1, nil, nil, nil, nil, nil, "c2", nil, nil)
	insertFact(t, db, center.Add(2*time.Minute).Format(time.RFC3339), "p", "m", 0, int64(99), int64(99), int64(99), nil, 9.0, "c3", nil, nil)
	// Partial usage: successful, but completion is unknown. Countable() excludes it
	// from the token/cost totals, so it must not be summed as a zero-completion row.
	insertFact(t, db, center.Add(3*time.Minute).Format(time.RFC3339), "p", "m", 1, int64(50), nil, nil, nil, 5.0, "c4", nil, nil)
	insertFact(t, db, center.Add(-48*time.Hour).Format(time.RFC3339), "old", "m", 1, int64(7), int64(3), int64(1), nil, 1.0, "old", nil, nil)

	service := Service{DB: db}
	all, err := service.Summary(nil)
	if err != nil {
		t.Fatal(err)
	}
	if all.Requests != 4 || all.PromptTokens != 17 || all.CompletionTokens != 8 || all.CachedTokens != 3 {
		t.Fatalf("all-time summary = %+v", all)
	}
	if all.Cost == nil || *all.Cost != 1.25 {
		t.Fatalf("all-time cost = %v, want 1.25", all.Cost)
	}

	windowed, err := service.Summary(&Window{From: center.Add(-time.Hour).UnixMilli(), To: center.Add(time.Hour).UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	if windowed.Requests != 3 || windowed.PromptTokens != 10 || windowed.Cost == nil || *windowed.Cost != 0.25 {
		t.Fatalf("windowed summary = %+v", windowed)
	}
}
