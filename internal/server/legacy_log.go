package server

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/heihei0299/pi-switch/internal/store"
)

// Legacy request-log compatibility (旧版本请求日志兼容):
// Go 重写前请求只追加到 requests.log（旧 camelCase 字段形状）；
// 新版统计只读 SQLite。启动时在后台把日志历史行幂等导入 SQLite，
// 新请求则由 logRequest 双写两处。requests.log 本身只读不写回。

const legacyBatchSize = 500

var legacyMu sync.Mutex

// legacyLogPath resolves requests.log next to the active config file:
// PI_SWITCH_CONFIG dir > ~/.pi-switch/requests.log.
func legacyLogPath() string {
	return filepath.Join(filepath.Dir(configPath()), "requests.log")
}

// legacySourceIdentity uses the operating-system file identity when it is
// available. It intentionally excludes size and mtime so appends keep the
// same migration stream; a replaced file gets a new identity on normal filesystems.
func legacySourceIdentity(fi os.FileInfo) string {
	sys := fi.Sys()
	if sys != nil {
		value := reflect.ValueOf(sys)
		if value.Kind() == reflect.Ptr && !value.IsNil() {
			value = value.Elem()
		}
		if value.Kind() != reflect.Struct {
			return fmt.Sprintf("%T", sys)
		}
		parts := make([]string, 0, 5)
		for _, name := range []string{"Dev", "Ino", "VolumeSerialNumber", "FileIndexHigh", "FileIndexLow"} {
			field := value.FieldByName(name)
			if field.IsValid() && field.CanInterface() {
				parts = append(parts, fmt.Sprintf("%s=%v", name, field.Interface()))
			}
		}
		if len(parts) > 0 {
			return fmt.Sprintf("%T:%s", sys, strings.Join(parts, ","))
		}
		return fmt.Sprintf("%T", sys)
	}
	return fmt.Sprintf("mtime:%d", fi.ModTime().UnixNano())
}

func legacyMigrationKey(path string, fi os.FileInfo) store.LegacyMigrationKey {
	return store.LegacyMigrationKey{
		Path:     path,
		Identity: legacySourceIdentity(fi),
		Version:  store.LegacyMigrationVersion,
	}
}

func legacyFileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// ensureLegacyImported is retained as the startup adapter. Stats and export
// handlers must not call it: reads are SQLite-only after IMP-10.
func ensureLegacyImported(db *sql.DB) {
	legacyMu.Lock()
	defer legacyMu.Unlock()
	if _, _, err := importLegacyPath(db, legacyLogPath()); err != nil {
		log.Printf("legacy requests.log import: %v", err)
	}
}

// ImportLegacyNow runs the migration synchronously for an explicit command or
// deterministic tests. It is separate from the asynchronous startup adapter.
func ImportLegacyNow() error {
	legacyMu.Lock()
	defer legacyMu.Unlock()
	db, err := store.Open(store.DBPath())
	if err != nil {
		return err
	}
	defer db.Close()
	_, _, err = importLegacyPath(db, legacyLogPath())
	return err
}

func importLegacyPath(db *sql.DB, path string) (imported, skipped int, err error) {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	// Callers serialize local importers; the store CAS protects the same source
	// when two processes share the database.
	key := legacyMigrationKey(path, fi)
	hash, err := legacyFileHash(path)
	if err != nil {
		return 0, 0, err
	}
	state, err := store.EnsureLegacyMigration(db, key, fi.Size(), fi.ModTime().UnixNano(), hash)
	if err != nil {
		return 0, 0, err
	}
	if state.CommittedOffset > fi.Size() || state.Hash != "" && state.Hash != hash && state.Size == fi.Size() {
		// The source was truncated or replaced in place. Preserve the old
		// stream and start a new durable generation instead of silently
		// reusing its committed offset.
		key.Identity = fmt.Sprintf("%s:replaced:%s", key.Identity, hash)
		state, err = store.EnsureLegacyMigration(db, key, fi.Size(), fi.ModTime().UnixNano(), hash)
		if err != nil {
			return 0, 0, err
		}
	}
	if state.Completed && state.CommittedOffset >= fi.Size() {
		return 0, 0, nil
	}
	imported, skipped, err = importLegacyLog(db, path, key, state, fi, hash)
	if err != nil {
		_ = store.MarkLegacyMigrationFailed(db, key, fi.Size(), fi.ModTime().UnixNano(), state.CommittedOffset)
	}
	return imported, skipped, err
}

// importLegacyLog commits complete-line batches together with their durable
// offsets. A final line without a newline is left uncommitted so a later append
// can resume it without losing or duplicating data.
func importLegacyLog(db *sql.DB, path string, key store.LegacyMigrationKey, state store.LegacyMigrationState, fi os.FileInfo, sourceHash string) (imported, skipped int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	if state.CommittedOffset > 0 {
		if _, err := f.Seek(state.CommittedOffset, io.SeekStart); err != nil {
			return 0, 0, err
		}
	}

	reader := bufio.NewReaderSize(f, 64*1024)
	offset := state.CommittedOffset
	batchStart := offset
	batchLines := 0
	badLines := int64(0)
	batch := make([]store.LegacyRequest, 0, legacyBatchSize)

	flush := func(completed bool) error {
		if batchLines == 0 && !completed {
			return nil
		}
		if err := store.CommitLegacyBatch(db, key, fi.Size(), fi.ModTime().UnixNano(), batchStart, offset, batch, badLines, completed, sourceHash); err != nil {
			return err
		}
		imported += len(batch)
		skipped += int(badLines)
		batch = batch[:0]
		badLines = 0
		batchLines = 0
		batchStart = offset
		return nil
	}

	for {
		lineStart := offset
		raw, readErr := reader.ReadBytes('\n')
		if len(raw) == 0 && errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return imported, skipped, readErr
		}
		if errors.Is(readErr, io.EOF) {
			// A partial tail is deliberately not counted or advanced.
			if err := flush(false); err != nil {
				return imported, skipped, err
			}
			if offset < fi.Size() {
				if err := store.CommitLegacyBatch(db, key, fi.Size(), fi.ModTime().UnixNano(), offset, offset, nil, 0, false, sourceHash); err != nil {
					return imported, skipped, err
				}
			}
			return imported, skipped, nil
		}

		offset += int64(len(raw))
		batchLines++
		line := strings.TrimSpace(string(bytes.TrimSuffix(raw, []byte{'\n'})))
		if line != "" {
			var value map[string]interface{}
			if json.Unmarshal([]byte(line), &value) != nil {
				badLines++
			} else if row, ok := parseLegacyRequest(value, lineStart); ok {
				batch = append(batch, row)
			} else {
				badLines++
			}
		}
		if batchLines >= legacyBatchSize {
			if err := flush(false); err != nil {
				return imported, skipped, err
			}
		}
	}

	if err := flush(true); err != nil {
		return imported, skipped, err
	}
	return imported, skipped, nil
}

func parseLegacyRequest(value map[string]interface{}, offset int64) (store.LegacyRequest, bool) {
	ts := legacyString(value, "ts")
	success, ok := value["ok"].(bool)
	if ts == "" || !ok {
		return store.LegacyRequest{}, false
	}
	return store.LegacyRequest{
		Offset:           offset,
		TS:               ts,
		Provider:         legacyString(value, "provider"),
		Model:            legacyString(value, "model"),
		Success:          success,
		PromptTokens:     legacyIntPointer(value, "promptTokens", "prompt_tokens"),
		CompletionTokens: legacyIntPointer(value, "completionTokens", "completion_tokens"),
		CachedTokens:     legacyIntPointer(value, "cachedTokens", "cached_tokens"),
		ReasoningTokens:  legacyIntPointer(value, "reasoningTokens", "reasoning_tokens"),
		Cost:             legacyFloatPointer(value, "costTotal", "cost", "total_cost"),
		ConversationID:   legacyOptionalString(value, "conversationId", "conversation_id"),
		ConversationName: legacyOptionalString(value, "conversationName", "conversation_name"),
		LatencyMs:        legacyIntPointer(value, "latencyMs", "latency_ms"),
	}, true
}

func legacyString(value map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if text, ok := value[key].(string); ok {
			return text
		}
	}
	return ""
}

func legacyOptionalString(value map[string]interface{}, keys ...string) *string {
	text := legacyString(value, keys...)
	if text == "" {
		return nil
	}
	return &text
}

func legacyIntPointer(value map[string]interface{}, keys ...string) *int64 {
	for _, key := range keys {
		switch number := value[key].(type) {
		case float64:
			converted := int64(number)
			return &converted
		case json.Number:
			converted, err := number.Int64()
			if err == nil {
				return &converted
			}
		}
	}
	return nil
}

func legacyFloatPointer(value map[string]interface{}, keys ...string) *float64 {
	for _, key := range keys {
		switch number := value[key].(type) {
		case float64:
			return &number
		case json.Number:
			converted, err := number.Float64()
			if err == nil {
				return &converted
			}
		}
	}
	return nil
}

// appendLegacyLog appends one old-shape JSON line to requests.log.
func appendLegacyLog(entry map[string]interface{}) error {
	path := legacyLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// legacyLogEntry builds the old-shape log line from a completed request.
func legacyLogEntry(ts, provider, model string, success bool, prompt, completion, cached, reasoning int, cost *float64, convID, convName string, status int, errMsg, upstreamURL string) map[string]interface{} {
	entry := map[string]interface{}{
		"ts": ts, "provider": provider, "model": model, "ok": success, "status": status,
		"upstreamUrl":  upstreamURL,
		"promptTokens": prompt, "completionTokens": completion, "cachedTokens": cached, "reasoningTokens": reasoning,
	}
	if errMsg != "" {
		entry["error"] = errMsg
	} else {
		entry["error"] = nil
	}
	if cost != nil {
		entry["costTotal"] = *cost
	}
	if convID != "" && convID != "unlabeled" {
		entry["conversationId"] = convID
	} else {
		entry["conversationId"] = nil
	}
	if convName != "" {
		entry["conversationName"] = convName
	} else {
		entry["conversationName"] = nil
	}
	return entry
}

// requestURLOf extracts the upstream URL from a client response.
func requestURLOf(resp *http.Response) string {
	if resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return ""
	}
	return resp.Request.URL.String()
}

// ImportLegacyOnStartup imports requests.log asynchronously. The listener is
// allowed to become healthy before the migration completes.
func ImportLegacyOnStartup() {
	go func() {
		db, err := store.GetDB()
		if err != nil {
			return
		}
		ensureLegacyImported(db)
	}()
}
