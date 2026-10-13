package stats

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"path/filepath"
	"strings"
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

func TestExportRequestsPreservesContractOrderUnknownsAndReadOnlyFacts(t *testing.T) {
	db := openStatsDB(t)
	insertFact(t, db, "2026-09-10T12:00:00Z", "supplier", "known", 1, int64(10), int64(5), int64(2), nil, 0.25, nil, nil, int64(12))
	insertFact(t, db, "2026-09-10T12:01:00Z", "supplier", "unknown", 0, nil, nil, nil, nil, nil, nil, nil, nil)
	if _, err := db.Exec(`UPDATE requests SET status = 502, error = 'upstream failed', upstream_url = 'https://supplier.example/v1' WHERE model = 'unknown'`); err != nil {
		t.Fatal(err)
	}
	service := Service{
		DB: db, Source: conversation.SourceSessionScan,
		Candidates: []conversation.Candidate{{ID: "session", Model: "known", LastActiveAt: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)}},
	}

	var jsonOutput bytes.Buffer
	if err := service.ExportRequests(&jsonOutput, "json"); err != nil {
		t.Fatal(err)
	}
	var requests []map[string]any
	if err := json.Unmarshal(jsonOutput.Bytes(), &requests); err != nil {
		t.Fatalf("exported JSON %q: %v", jsonOutput.String(), err)
	}
	if len(requests) != 2 || requests[0]["model"] != "known" || requests[1]["model"] != "unknown" {
		t.Fatalf("exported requests/order = %+v", requests)
	}
	if requests[0]["ts"] != "2026-09-10T12:00:00Z" || requests[0]["timestamp"] != requests[0]["ts"] || requests[0]["costTotal"] != 0.25 || requests[0]["cost"] != requests[0]["costTotal"] {
		t.Fatalf("JSON export does not preserve timestamp/cost aliases: %+v", requests[0])
	}
	if requests[0]["conversationId"] != nil {
		t.Fatalf("JSON export inferred a session attribution: %+v", requests[0])
	}
	if requests[1]["prompt_tokens"] != nil || requests[1]["cachedTokens"] != nil || requests[1]["cost"] != nil || requests[1]["ok"] != false || requests[1]["status"] != float64(502) || requests[1]["error"] != "upstream failed" || requests[1]["upstreamUrl"] != "https://supplier.example/v1" {
		t.Fatalf("JSON export fabricated unknowns or dropped failure aliases: %+v", requests[1])
	}

	var csvOutput bytes.Buffer
	if err := service.ExportRequests(&csvOutput, "csv"); err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(strings.NewReader(csvOutput.String())).ReadAll()
	if err != nil || len(records) != 3 {
		t.Fatalf("exported CSV records=%v, err=%v", records, err)
	}
	wantHeader := "timestamp,ok,provider,model,status,latency_ms,error,retry,skipped,converted,upstream_url,promptTokens,completionTokens,cachedTokens,reasoningTokens,conversationId,conversationName,costTotal,cost,cached_tokens,reasoning_tokens"
	if strings.Join(records[0], ",") != wantHeader {
		t.Fatalf("CSV header = %q, want %q", strings.Join(records[0], ","), wantHeader)
	}
	columns := make(map[string]int, len(records[0]))
	for i, column := range records[0] {
		columns[column] = i
	}
	if records[1][columns["model"]] != "known" || records[2][columns["model"]] != "unknown" || records[2][columns["promptTokens"]] != "" || records[2][columns["cachedTokens"]] != "" || records[2][columns["cost"]] != "" || records[2][columns["status"]] != "502" || records[2][columns["error"]] != "upstream failed" {
		t.Fatalf("CSV order/nullability/failure facts = %v", records)
	}
	if err := service.ExportRequests(&bytes.Buffer{}, "xml"); err == nil {
		t.Fatal("unsupported export format was accepted")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM requests`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("export changed request facts: count=%d, err=%v", count, err)
	}
}
