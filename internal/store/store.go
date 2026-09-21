package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

// RequestRow mirrors the requests table row for stats.
type RequestRow struct {
	TS               string   `json:"ts"`
	Provider         string   `json:"provider"`
	Model            string   `json:"model"`
	Success          bool     `json:"success"`
	PromptTokens     int      `json:"prompt_tokens"`
	CompletionTokens int      `json:"completion_tokens"`
	CachedTokens     int      `json:"cached_tokens"`
	Cost             *float64 `json:"cost"`
	ConversationID   string   `json:"conversation_id"`
	LatencyMs        int64    `json:"latency_ms"`
}

// DBPath resolves the SQLite path: PI_SWITCH_DB env > ~/.pi-switch/requests.db > /tmp fallback.
func DBPath() string {
	if p := os.Getenv("PI_SWITCH_DB"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "/tmp/pi-switch-requests.db"
	}
	return filepath.Join(home, ".pi-switch", "requests.db")
}

// Open opens or creates the SQLite DB, ensures table exists and migrates columns.
func Open(path string) (*sql.DB, error) {
	if path == "" {
		path = DBPath()
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Serialize short SQLite writer contention (startup migration, request
	// logging, and concurrent explicit import triggers) instead of surfacing
	// SQLITE_BUSY to an otherwise idempotent caller.
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureTable(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func ensureTable(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS requests (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ts TEXT,
		provider TEXT,
		model TEXT,
		success INTEGER,
		prompt_tokens INTEGER,
		completion_tokens INTEGER,
		cached_tokens INTEGER,
		reasoning_tokens INTEGER,
		cost REAL,
		conversation_id TEXT,
		conversation_name TEXT,
		latency_ms INTEGER
	)`); err != nil {
		return err
	}

	rows, err := db.Query(`PRAGMA table_info(requests)`)
	if err != nil {
		return err
	}
	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			_ = rows.Close()
			return err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	columnTypes := []struct{ name, typ string }{
		{"reasoning_tokens", "INTEGER"},
		{"cost", "REAL"},
		{"conversation_name", "TEXT"},
		{"latency_ms", "INTEGER"},
		{"legacy_source", "TEXT"},
		{"legacy_identity", "TEXT"},
		{"legacy_offset", "INTEGER"},
	}
	for _, column := range columnTypes {
		if columns[column.name] {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE requests ADD COLUMN ` + column.name + ` ` + column.typ); err != nil {
			return fmt.Errorf("migrate requests.%s: %w", column.name, err)
		}
	}
	return ensureLegacyMigrationSchema(db)
}

// InsertRequest inserts one request row. cost may be nil (unknown).
func InsertRequest(db *sql.DB, provider, model string, success bool, prompt, completion, cached int, cost *float64, conversationID string, latencyMs int64, ts string) error {
	succ := 0
	if success {
		succ = 1
	}
	var costVal interface{}
	if cost != nil {
		costVal = *cost
	} else {
		costVal = nil
	}
	_, err := db.Exec(`INSERT INTO requests(ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,cost,conversation_id,latency_ms) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		ts, provider, model, succ, prompt, completion, cached, costVal, conversationID, latencyMs)
	return err
}

// QueryRecent returns the newest N rows, newest first.
func QueryRecent(db *sql.DB, limit int) ([]RequestRow, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.Query(`SELECT ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,cost,conversation_id,latency_ms FROM requests ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RequestRow
	for rows.Next() {
		var r RequestRow
		var succ int
		var cost sql.NullFloat64
		var ts, provider, model, convID sql.NullString
		var pt, ct, cached sql.NullInt64
		var latency sql.NullInt64
		if err := rows.Scan(&ts, &provider, &model, &succ, &pt, &ct, &cached, &cost, &convID, &latency); err != nil {
			return nil, err
		}
		r.TS = ts.String
		r.Provider = provider.String
		r.Model = model.String
		r.Success = succ == 1
		if pt.Valid {
			r.PromptTokens = int(pt.Int64)
		}
		if ct.Valid {
			r.CompletionTokens = int(ct.Int64)
		}
		if cached.Valid {
			r.CachedTokens = int(cached.Int64)
		}
		if cost.Valid {
			v := cost.Float64
			r.Cost = &v
		}
		r.ConversationID = convID.String
		if latency.Valid {
			r.LatencyMs = latency.Int64
		}
		out = append(out, r)
	}
	if out == nil {
		out = []RequestRow{}
	}
	return out, nil
}

var (
	muDB       sync.Mutex
	cachedDB   *sql.DB
	cachedPath string
)

func GetDB() (*sql.DB, error) {
	muDB.Lock()
	defer muDB.Unlock()
	path := DBPath()
	if cachedDB != nil && cachedPath == path {
		if err := cachedDB.Ping(); err == nil {
			return cachedDB, nil
		}
		_ = cachedDB.Close()
		cachedDB = nil
	}
	if cachedDB != nil {
		_ = cachedDB.Close()
		cachedDB = nil
	}
	db, err := Open(path)
	if err != nil {
		return nil, err
	}
	cachedDB = db
	cachedPath = path
	return db, nil
}

func Close() {
	muDB.Lock()
	defer muDB.Unlock()
	if cachedDB != nil {
		_ = cachedDB.Close()
		cachedDB = nil
		cachedPath = ""
	}
}

func ResetForTest(path string) (*sql.DB, error) {
	muDB.Lock()
	if cachedDB != nil {
		_ = cachedDB.Close()
		cachedDB = nil
		cachedPath = ""
	}
	muDB.Unlock()
	_ = os.Remove(path)
	db, err := Open(path)
	if err != nil {
		return nil, err
	}
	muDB.Lock()
	cachedDB = db
	cachedPath = path
	muDB.Unlock()
	return db, nil
}
