package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenReturnsFailedAlterTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requests.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE requests (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts TEXT,
		provider TEXT,
		model TEXT,
		success INTEGER,
		prompt_tokens INTEGER,
		completion_tokens INTEGER,
		cached_tokens INTEGER,
		reasoning_tokens TEXT GENERATED ALWAYS AS ('x') VIRTUAL
	)`)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	opened, err := Open(path)
	if err == nil {
		opened.Close()
		t.Fatal("Open succeeded after ALTER TABLE failed")
	}
}

func TestOpenReportsMalformedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requests.db")
	if err := os.WriteFile(path, []byte("not sqlite"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("Open succeeded for malformed database")
	}
}

func TestRequestErrorFactMigrationPreservesOldRowsAndUnknowns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requests.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE requests (
		id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT, provider TEXT, model TEXT, success INTEGER,
		prompt_tokens INTEGER, completion_tokens INTEGER, cached_tokens INTEGER, reasoning_tokens INTEGER,
		cost REAL, conversation_id TEXT, conversation_name TEXT, latency_ms INTEGER
	)`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO requests(ts,provider,model,success,prompt_tokens) VALUES ('2026-09-10T12:00:00Z','old','m',0,100)`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		db, err = Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var count, success, prompt int64
		var provider string
		var status sql.NullInt64
		var errMsg, upstreamURL sql.NullString
		err = db.QueryRow(`SELECT COUNT(*),provider,success,prompt_tokens,status,error,upstream_url FROM requests`).Scan(&count, &provider, &success, &prompt, &status, &errMsg, &upstreamURL)
		if err != nil || count != 1 || provider != "old" || success != 0 || prompt != 100 || status.Valid || errMsg.Valid || upstreamURL.Valid {
			t.Errorf("migration changed old facts: count=%d provider=%s success=%d prompt=%d status=%v error=%v URL=%v err=%v", count, provider, success, prompt, status, errMsg, upstreamURL, err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOpenKeepsDatabasePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports permission bits differently")
	}
	path := filepath.Join(t.TempDir(), "requests.db")
	assertPrivate := func() {
		t.Helper()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != 0600 {
			t.Fatalf("database mode = %04o, want 0600", mode)
		}
	}

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	assertPrivate()
	if _, err := db.Exec(`INSERT INTO requests(provider,model) VALUES ('private','fixture')`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	assertPrivate()
	var provider, model string
	if err := db.QueryRow(`SELECT provider,model FROM requests`).Scan(&provider, &model); err != nil {
		t.Fatal(err)
	}
	if provider != "private" || model != "fixture" {
		t.Fatalf("existing row = %q/%q, want private/fixture", provider, model)
	}
}

func TestOpenKeepsFileURIDatabasePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports permission bits differently")
	}
	path := filepath.Join(t.TempDir(), "requests.db")
	db, err := Open("file:" + path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("URI database mode = %04o, want 0600", mode)
	}
}

func TestOpenSupportsInMemoryDatabases(t *testing.T) {
	for _, path := range []string{":memory:", "file::memory:", "file:memdb?mode=memory&cache=shared"} {
		t.Run(path, func(t *testing.T) {
			db, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM requests`).Scan(&count); err != nil {
				t.Fatal(err)
			}
		})
	}
}
