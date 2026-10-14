package sqlanalytics

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/heihei0299/pi-switch/internal/store"
	"github.com/heihei0299/pi-switch/internal/usecase/analytics"
)

func openRepositoryDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "requests.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func insertRepositoryFact(t *testing.T, db *sql.DB, ts, model string, success any, prompt any) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO requests(ts,model,success,prompt_tokens) VALUES(?,?,?,?)`, ts, model, success, prompt); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryForEachUsesMillisecondRightOpenWindowAndPreservesNulls(t *testing.T) {
	db := openRepositoryDB(t)
	center := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	inside := center.Add(250 * time.Millisecond)
	insertRepositoryFact(t, db, inside.Format(time.RFC3339Nano), "inside", 1, 10)
	insertRepositoryFact(t, db, center.Add(500*time.Millisecond).Format(time.RFC3339Nano), "edge", 1, nil)

	var got []analytics.RequestFact
	err := New(db).ForEachFact(&analytics.Window{From: center.UnixMilli(), To: center.Add(500 * time.Millisecond).UnixMilli()}, analytics.OldestFirst, func(fact analytics.RequestFact) error {
		got = append(got, fact)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Model == nil || *got[0].Model != "inside" || got[0].PromptTokens == nil || *got[0].PromptTokens != 10 {
		t.Fatalf("window facts = %+v", got)
	}
	if got[0].CompletionTokens != nil || got[0].Cost != nil || got[0].ConversationID != nil {
		t.Fatalf("unknown facts became known: %+v", got[0])
	}
}

func TestRepositoryPageFactsIsNewestFirstAndStableByID(t *testing.T) {
	db := openRepositoryDB(t)
	insertRepositoryFact(t, db, "2026-09-10T12:00:00Z", "first", 1, 1)
	insertRepositoryFact(t, db, "2026-09-10T12:01:00Z", "second", 1, 2)

	first, err := New(db).PageFacts(nil, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(db).PageFacts(nil, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].Model == nil || *first[0].Model != "second" || second[0].Model == nil || *second[0].Model != "first" {
		t.Fatalf("pages = %+v / %+v", first, second)
	}
}
