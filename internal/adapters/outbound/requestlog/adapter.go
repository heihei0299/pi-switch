package requestlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	requestlog "github.com/heihei0299/pi-switch/internal/usecase/requestlog"
)

type SQLite struct {
	DB *sql.DB
}

func NewSQLite(db *sql.DB) SQLite { return SQLite{DB: db} }

func (s SQLite) Insert(ctx context.Context, record requestlog.Record) error {
	if s.DB == nil {
		return fmt.Errorf("request log database unavailable")
	}
	prompt, completion, cached, reasoning := record.Usage.NullableTokens()
	var cost any
	if record.Cost != nil {
		cost = *record.Cost
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO requests(ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id,conversation_name,latency_ms,status,error,upstream_url) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,NULLIF(?,''),NULLIF(?,''))`,
		record.TS, record.Provider, record.Model, boolInt(record.Success), prompt, completion, cached, reasoning, cost, record.ConversationID, record.ConversationName, record.LatencyMs, record.Status, record.Error, record.UpstreamURL)
	if err != nil {
		return fmt.Errorf("request log insert: %w", err)
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type LegacyFile struct {
	Path string
}

func NewLegacyFile(path string) LegacyFile { return LegacyFile{Path: path} }

func (f LegacyFile) Append(ctx context.Context, record requestlog.Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0755); err != nil {
		return err
	}
	payload, err := json.Marshal(legacyEntry(record))
	if err != nil {
		return err
	}
	file, err := os.OpenFile(f.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(payload, '\n')); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func legacyEntry(record requestlog.Record) map[string]interface{} {
	prompt, completion, cached, reasoning := record.Usage.NullableTokens()
	entry := map[string]interface{}{
		"ts": record.TS, "provider": record.Provider, "model": record.Model, "ok": record.Success, "status": record.Status,
		"upstreamUrl":  record.UpstreamURL,
		"promptTokens": prompt, "completionTokens": completion, "cachedTokens": cached, "reasoningTokens": reasoning,
	}
	if record.Error != "" {
		entry["error"] = record.Error
	} else {
		entry["error"] = nil
	}
	if record.Cost != nil {
		entry["costTotal"] = *record.Cost
	}
	if record.ConversationID != "" && record.ConversationID != "unlabeled" {
		entry["conversationId"] = record.ConversationID
	} else {
		entry["conversationId"] = nil
	}
	if record.ConversationName != "" {
		entry["conversationName"] = record.ConversationName
	} else {
		entry["conversationName"] = nil
	}
	return entry
}

type UnavailableDatabase struct{ Err error }

func (d UnavailableDatabase) Insert(context.Context, requestlog.Record) error {
	if d.Err != nil {
		return d.Err
	}
	return fmt.Errorf("request log database unavailable")
}
