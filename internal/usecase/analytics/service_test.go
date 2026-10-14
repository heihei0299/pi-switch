package analytics

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/heihei0299/pi-switch/internal/domain/conversation"
)

type factMemory []RequestFact

func (rows factMemory) ForEachFact(window *Window, order Order, visit func(RequestFact) error) error {
	for _, row := range rows.selected(window, order) {
		if err := visit(row); err != nil {
			return err
		}
	}
	return nil
}

func (rows factMemory) PageFacts(window *Window, page, limit int) ([]RequestFact, error) {
	if page < 0 {
		page = 0
	}
	if limit <= 0 {
		limit = 50
	}
	selected := rows.selected(window, NewestFirst)
	start := min(page*limit, len(selected))
	end := min(start+limit, len(selected))
	return selected[start:end], nil
}

func (rows factMemory) selected(window *Window, order Order) []RequestFact {
	selected := make([]RequestFact, 0, len(rows))
	for _, row := range rows {
		if window != nil {
			if row.TS == nil {
				continue
			}
			timestamp, err := parseTimestamp(*row.TS)
			if err != nil || timestamp.UnixMilli() < window.From || timestamp.UnixMilli() >= window.To {
				continue
			}
		}
		selected = append(selected, row)
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if order == OldestFirst {
			return selected[i].ID < selected[j].ID
		}
		return selected[i].ID > selected[j].ID
	})
	return selected
}

type countedFactRepository struct {
	rows      factMemory
	forEach   int
	pageFacts int
}

func (r *countedFactRepository) ForEachFact(window *Window, order Order, visit func(RequestFact) error) error {
	r.forEach++
	return r.rows.ForEachFact(window, order, visit)
}

func (r *countedFactRepository) PageFacts(window *Window, page, limit int) ([]RequestFact, error) {
	r.pageFacts++
	return r.rows.PageFacts(window, page, limit)
}

func ptr[T any](value T) *T { return &value }

func TestStatsServiceUsesWindowAndPreservesNullableFacts(t *testing.T) {
	center := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	rows := factMemory{
		{ID: 1, TS: ptr(center.Format(time.RFC3339)), Provider: ptr("p"), Model: ptr("m"), Success: ptr(true), PromptTokens: ptr(int64(10)), CompletionTokens: ptr(int64(5)), CachedTokens: ptr(int64(2)), Cost: ptr(0.25), ConversationID: ptr("c1"), ConversationName: ptr("named"), LatencyMs: ptr(int64(12))},
		{ID: 2, TS: ptr(center.Add(-48 * time.Hour).Format(time.RFC3339)), Provider: ptr("old"), Model: ptr("old-model"), Success: ptr(true), PromptTokens: ptr(int64(99)), CompletionTokens: ptr(int64(1)), CachedTokens: ptr(int64(0)), Cost: ptr(9.0), ConversationID: ptr("old"), LatencyMs: ptr(int64(99))},
		{ID: 3, TS: ptr(center.Add(time.Minute).Format(time.RFC3339)), Provider: ptr("failed"), Model: ptr("m"), Success: ptr(false), CompletionTokens: ptr(int64(3)), Cost: ptr(3.0), ConversationID: ptr("c2")},
	}
	service := NewService(rows, conversation.SourceProxy, nil)
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

func TestStatsServiceSharesConversationAttributionAcrossEndpoints(t *testing.T) {
	service := NewService(factMemory{{ID: 1, TS: ptr("2026-09-10T12:00:00Z"), Provider: ptr("p"), Model: ptr("m"), Success: ptr(true), PromptTokens: ptr(int64(10)), CompletionTokens: ptr(int64(5))}}, conversation.SourceProxy, nil)
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

func TestStatsAndConversationsReusesAggregationForSameWindow(t *testing.T) {
	base := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	repository := &countedFactRepository{rows: factMemory{
		{ID: 1, TS: ptr(base.Format(time.RFC3339)), Success: ptr(true), PromptTokens: ptr(int64(1)), CompletionTokens: ptr(int64(1)), ConversationID: ptr("c1")},
		{ID: 2, TS: ptr(base.Add(time.Minute).Format(time.RFC3339)), Success: ptr(true), PromptTokens: ptr(int64(1)), CompletionTokens: ptr(int64(1)), ConversationID: ptr("c2")},
	}}
	service := NewService(repository, conversation.SourceProxy, nil)
	window := &Window{From: base.UnixMilli(), To: base.Add(time.Hour).UnixMilli()}
	stats, conversations, total, err := service.StatsAndConversations(window, window, 0, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalRequests != 2 || repository.forEach != 1 || repository.pageFacts != 1 {
		t.Fatalf("same-window reads = stats %d, fact scans %d, page reads %d", stats.TotalRequests, repository.forEach, repository.pageFacts)
	}
	if total != 2 || len(conversations) != 1 || conversations[0].ConversationID != "c1" {
		t.Fatalf("conversation page = total %d, values %+v", total, conversations)
	}
}

func TestStatsAndConversationsScansDistinctWindowsSeparately(t *testing.T) {
	base := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	repository := &countedFactRepository{rows: factMemory{
		{ID: 1, TS: ptr(base.Format(time.RFC3339)), Success: ptr(true), PromptTokens: ptr(int64(1)), CompletionTokens: ptr(int64(1)), ConversationID: ptr("stats")},
		{ID: 2, TS: ptr(base.Add(24 * time.Hour).Format(time.RFC3339)), Success: ptr(true), PromptTokens: ptr(int64(1)), CompletionTokens: ptr(int64(1)), ConversationID: ptr("conversations")},
	}}
	service := NewService(repository, conversation.SourceProxy, nil)
	statsWindow := &Window{From: base.UnixMilli(), To: base.Add(time.Hour).UnixMilli()}
	conversationWindow := &Window{From: base.Add(24 * time.Hour).UnixMilli(), To: base.Add(25 * time.Hour).UnixMilli()}
	stats, conversations, total, err := service.StatsAndConversations(statsWindow, conversationWindow, 0, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalRequests != 1 || repository.forEach != 2 || repository.pageFacts != 1 {
		t.Fatalf("different-window reads = stats %d, fact scans %d, page reads %d", stats.TotalRequests, repository.forEach, repository.pageFacts)
	}
	if total != 1 || len(conversations) != 1 || conversations[0].ConversationID != "conversations" {
		t.Fatalf("conversation page = total %d, values %+v", total, conversations)
	}
}

func TestStatsServicePaginationIsDeterministic(t *testing.T) {
	base := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	service := NewService(factMemory{
		{ID: 1, TS: ptr(base.Add(2 * time.Minute).Format(time.RFC3339)), Success: ptr(true)},
		{ID: 2, TS: ptr(base.Add(time.Minute).Format(time.RFC3339)), Success: ptr(true)},
	}, conversation.SourceProxy, nil)
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
	if *first.RecentRequests[0].TS != base.Add(time.Minute).Format(time.RFC3339) || *second.RecentRequests[0].TS != base.Add(2*time.Minute).Format(time.RFC3339) {
		t.Fatalf("pages are not deterministic by request id: first=%s second=%s", *first.RecentRequests[0].TS, *second.RecentRequests[0].TS)
	}
}

func TestSummaryAggregatesSuccessfulRequests(t *testing.T) {
	center := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	service := NewService(factMemory{
		{ID: 1, TS: ptr(center.Format(time.RFC3339)), Success: ptr(true), PromptTokens: ptr(int64(10)), CompletionTokens: ptr(int64(5)), CachedTokens: ptr(int64(2)), Cost: ptr(0.25)},
		{ID: 2, TS: ptr(center.Add(time.Minute).Format(time.RFC3339)), Success: ptr(true)},
		{ID: 3, TS: ptr(center.Add(2 * time.Minute).Format(time.RFC3339)), Success: ptr(false), PromptTokens: ptr(int64(99)), CompletionTokens: ptr(int64(99)), CachedTokens: ptr(int64(99)), Cost: ptr(9.0)},
		{ID: 4, TS: ptr(center.Add(3 * time.Minute).Format(time.RFC3339)), Success: ptr(true), PromptTokens: ptr(int64(50)), Cost: ptr(5.0)},
		{ID: 5, TS: ptr(center.Add(-48 * time.Hour).Format(time.RFC3339)), Success: ptr(true), PromptTokens: ptr(int64(7)), CompletionTokens: ptr(int64(3)), CachedTokens: ptr(int64(1)), Cost: ptr(1.0)},
	}, conversation.SourceOff, nil)
	all, err := service.Summary(nil)
	if err != nil {
		t.Fatal(err)
	}
	if all.Requests != 4 || all.PromptTokens != 17 || all.CompletionTokens != 8 || all.CachedTokens != 3 || all.Cost == nil || *all.Cost != 1.25 {
		t.Fatalf("all-time summary = %+v", all)
	}
	windowed, err := service.Summary(&Window{From: center.Add(-time.Hour).UnixMilli(), To: center.Add(time.Hour).UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	if windowed.Requests != 3 || windowed.PromptTokens != 10 || windowed.Cost == nil || *windowed.Cost != 0.25 {
		t.Fatalf("windowed summary = %+v", windowed)
	}
}

func TestExportRequestsPreservesAliasesUnknownsAndOrder(t *testing.T) {
	service := NewService(factMemory{
		{ID: 1, TS: ptr("2026-09-10T12:00:00Z"), Provider: ptr("supplier"), Model: ptr("known"), Success: ptr(true), PromptTokens: ptr(int64(10)), CompletionTokens: ptr(int64(5)), CachedTokens: ptr(int64(2)), Cost: ptr(0.25), LatencyMs: ptr(int64(12))},
		{ID: 2, TS: ptr("2026-09-10T12:01:00Z"), Provider: ptr("supplier"), Model: ptr("unknown"), Success: ptr(false), Status: ptr(int64(502)), Error: ptr("upstream failed"), UpstreamURL: ptr("https://supplier.example/v1")},
	}, conversation.SourceSessionScan, []conversation.Candidate{{ID: "session", Model: "known", LastActiveAt: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)}})

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
	if records[1][3] != "known" || records[2][3] != "unknown" || records[2][11] != "" || records[2][18] != "" || records[2][4] != "502" || records[2][6] != "upstream failed" {
		t.Fatalf("CSV order/nullability/failure facts = %v", records)
	}
	if err := service.ExportRequests(&bytes.Buffer{}, "xml"); err == nil {
		t.Fatal("unsupported export format was accepted")
	}
}

func TestServiceReturnsRepositoryErrors(t *testing.T) {
	service := NewService(failingRepository{err: fmt.Errorf("read failed")}, conversation.SourceOff, nil)
	if _, err := service.Summary(nil); err == nil || err.Error() != "read failed" {
		t.Fatalf("repository error = %v", err)
	}
}

type failingRepository struct{ err error }

func (r failingRepository) ForEachFact(*Window, Order, func(RequestFact) error) error { return r.err }
func (r failingRepository) PageFacts(*Window, int, int) ([]RequestFact, error)        { return nil, r.err }
