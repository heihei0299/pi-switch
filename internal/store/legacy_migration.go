package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const LegacyMigrationVersion = 1

var ErrLegacyMigrationConflict = errors.New("legacy migration state changed concurrently")

type LegacyMigrationKey struct {
	Path     string
	Identity string
	Version  int
}

type LegacyMigrationState struct {
	Key             LegacyMigrationKey
	Size            int64
	Mtime           int64
	Hash            string
	CommittedOffset int64
	BadLines        int64
	Completed       bool
	Failed          bool
	Status          string
	UpdatedAt       string
}

type LegacyRequest struct {
	Offset           int64
	TS               string
	Provider         string
	Model            string
	Success          bool
	PromptTokens     *int64
	CompletionTokens *int64
	CachedTokens     *int64
	ReasoningTokens  *int64
	Cost             *float64
	ConversationID   *string
	ConversationName *string
	LatencyMs        *int64
}

func ensureLegacyMigrationSchema(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS legacy_migrations (
		source_path TEXT NOT NULL,
		source_identity TEXT NOT NULL,
		version INTEGER NOT NULL,
		size INTEGER NOT NULL DEFAULT 0,
		mtime INTEGER NOT NULL DEFAULT 0,
		source_hash TEXT NOT NULL DEFAULT '',
		committed_offset INTEGER NOT NULL DEFAULT 0,
		bad_lines INTEGER NOT NULL DEFAULT 0,
		completed INTEGER NOT NULL DEFAULT 0,
		failed INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'running',
		updated_at TEXT NOT NULL,
		PRIMARY KEY (source_path, source_identity, version)
	)`); err != nil {
		return err
	}
	if err := ensureLegacyMigrationHashColumn(db); err != nil {
		return err
	}
	_, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_requests_legacy_source_offset
		ON requests(legacy_source, legacy_identity, legacy_offset)
		WHERE legacy_source IS NOT NULL AND legacy_identity IS NOT NULL AND legacy_offset IS NOT NULL`)
	return err
}

func ensureLegacyMigrationHashColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(legacy_migrations)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == "source_hash" {
			return nil
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE legacy_migrations ADD COLUMN source_hash TEXT NOT NULL DEFAULT ''`)
	return err
}

func EnsureLegacyMigration(db *sql.DB, key LegacyMigrationKey, size, mtime int64, sourceHash ...string) (LegacyMigrationState, error) {
	if key.Version == 0 {
		key.Version = LegacyMigrationVersion
	}
	hash := ""
	if len(sourceHash) > 0 {
		hash = sourceHash[0]
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO legacy_migrations
		(source_path,source_identity,version,size,mtime,source_hash,committed_offset,bad_lines,completed,failed,status,updated_at)
		VALUES (?,?,?,?,?,?,0,0,0,0,'running',?)`,
		key.Path, key.Identity, key.Version, size, mtime, hash, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return LegacyMigrationState{}, err
	}
	return LoadLegacyMigration(db, key)
}

func LoadLegacyMigration(db *sql.DB, key LegacyMigrationKey) (LegacyMigrationState, error) {
	if key.Version == 0 {
		key.Version = LegacyMigrationVersion
	}
	var state LegacyMigrationState
	var completed, failed int
	state.Key = key
	err := db.QueryRow(`SELECT size,mtime,source_hash,committed_offset,bad_lines,completed,failed,status,updated_at
		FROM legacy_migrations WHERE source_path=? AND source_identity=? AND version=?`,
		key.Path, key.Identity, key.Version).Scan(
		&state.Size, &state.Mtime, &state.Hash, &state.CommittedOffset, &state.BadLines,
		&completed, &failed, &state.Status, &state.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return LegacyMigrationState{}, sql.ErrNoRows
	}
	if err != nil {
		return LegacyMigrationState{}, err
	}
	state.Completed = completed != 0
	state.Failed = failed != 0
	return state, nil
}

// CommitLegacyBatch atomically inserts a batch of legacy requests and advances
// the corresponding source offset. A stale expectedOffset is rejected so two
// importers cannot commit the same source range concurrently.
func MarkLegacyMigrationFailed(db *sql.DB, key LegacyMigrationKey, size, mtime, offset int64) error {
	if key.Version == 0 {
		key.Version = LegacyMigrationVersion
	}
	_, err := db.Exec(`UPDATE legacy_migrations SET size=?,mtime=?,failed=1,completed=0,status='failed',updated_at=?
		WHERE source_path=? AND source_identity=? AND version=? AND committed_offset=?`,
		size, mtime, time.Now().UTC().Format(time.RFC3339Nano), key.Path, key.Identity, key.Version, offset)
	return err
}

func CommitLegacyBatch(db *sql.DB, key LegacyMigrationKey, size, mtime, expectedOffset, nextOffset int64, rows []LegacyRequest, badLines int64, completed bool, sourceHash ...string) error {
	if key.Version == 0 {
		key.Version = LegacyMigrationVersion
	}
	hash := ""
	if len(sourceHash) > 0 {
		hash = sourceHash[0]
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	rollback := func(e error) error {
		_ = tx.Rollback()
		return e
	}
	for _, row := range rows {
		success := 0
		if row.Success {
			success = 1
		}
		if _, err := tx.Exec(`INSERT INTO requests(
			ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,
			conversation_id,conversation_name,latency_ms,legacy_source,legacy_identity,legacy_offset)
			SELECT ?,?,?,?,?,?,?,?,?,?,?,?,?,?,?
			WHERE NOT EXISTS (
				SELECT 1 FROM requests existing
				WHERE existing.legacy_source IS NULL
				  AND existing.ts IS ? AND existing.provider IS ? AND existing.model IS ?
				  AND existing.success IS ? AND existing.prompt_tokens IS ? AND existing.completion_tokens IS ?
			)`,
			row.TS, row.Provider, row.Model, success, nullableInt(row.PromptTokens),
			nullableInt(row.CompletionTokens), nullableInt(row.CachedTokens), nullableInt(row.ReasoningTokens),
			nullableFloat(row.Cost), nullableString(row.ConversationID), nullableString(row.ConversationName),
			nullableInt(row.LatencyMs), key.Path, key.Identity, row.Offset,
			row.TS, row.Provider, row.Model, success, nullableInt(row.PromptTokens), nullableInt(row.CompletionTokens)); err != nil {
			return rollback(err)
		}
	}
	completedValue, failedValue := 0, 0
	status := "running"
	if completed {
		completedValue = 1
		status = "completed"
	}
	result, err := tx.Exec(`UPDATE legacy_migrations SET
		size=?,mtime=?,source_hash=CASE WHEN ? <> '' THEN ? ELSE source_hash END,
		committed_offset=?,bad_lines=bad_lines+?,completed=?,failed=?,status=?,updated_at=?
		WHERE source_path=? AND source_identity=? AND version=? AND committed_offset=?`,
		size, mtime, hash, hash, nextOffset, badLines, completedValue, failedValue, status,
		time.Now().UTC().Format(time.RFC3339Nano), key.Path, key.Identity, key.Version, expectedOffset)
	if err != nil {
		return rollback(err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return rollback(err)
	}
	if changed != 1 {
		return rollback(fmt.Errorf("%w: expected offset %d", ErrLegacyMigrationConflict, expectedOffset))
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func nullableInt(value *int64) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func nullableFloat(value *float64) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func nullableString(value *string) interface{} {
	if value == nil {
		return nil
	}
	return *value
}
