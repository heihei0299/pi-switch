package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestLegacyMigrationBatchAdvancesOffsetWithRows(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "requests.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := LegacyMigrationKey{Path: "/tmp/requests.log", Identity: "dev:1", Version: LegacyMigrationVersion}
	if _, err := EnsureLegacyMigration(db, key, 20, 1, "hash-before"); err != nil {
		t.Fatal(err)
	}
	prompt := int64(10)
	if err := CommitLegacyBatch(db, key, 20, 1, 0, 20, []LegacyRequest{{
		Offset:       0,
		TS:           "2026-09-10T00:00:00Z",
		Provider:     "p",
		Model:        "m",
		Success:      true,
		PromptTokens: &prompt,
	}}, 2, true, "hash-after"); err != nil {
		t.Fatal(err)
	}
	state, err := LoadLegacyMigration(db, key)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Completed || state.CommittedOffset != 20 || state.BadLines != 2 || state.Hash != "hash-after" {
		t.Fatalf("unexpected migration state: %+v", state)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM requests WHERE legacy_source=? AND legacy_identity=?`, key.Path, key.Identity).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("legacy rows=%d, want 1", count)
	}
	var stored sql.NullInt64
	if err := db.QueryRow(`SELECT prompt_tokens FROM requests WHERE legacy_source=?`, key.Path).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !stored.Valid || stored.Int64 != prompt {
		t.Fatalf("prompt token=%+v, want valid %d", stored, prompt)
	}
}

func TestLegacyMigrationRejectsStaleOffsetAtomically(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "requests.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := LegacyMigrationKey{Path: "/tmp/requests.log", Identity: "dev:2", Version: LegacyMigrationVersion}
	if _, err := EnsureLegacyMigration(db, key, 10, 1); err != nil {
		t.Fatal(err)
	}
	err = CommitLegacyBatch(db, key, 10, 1, 5, 10, []LegacyRequest{{Offset: 5, TS: "ts", Success: true}}, 0, false)
	if !errors.Is(err, ErrLegacyMigrationConflict) {
		t.Fatalf("error=%v, want conflict", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM requests WHERE legacy_identity=?`, key.Identity).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stale batch inserted %d rows", count)
	}
}
