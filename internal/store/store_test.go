package store

import (
	"database/sql"
	"os"
	"path/filepath"
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
