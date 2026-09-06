package server

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/heihei0299/pi-switch/internal/store"
)

// Legacy request-log compatibility (旧版本请求日志兼容):
// Go 重写前请求只追加到 requests.log（旧 camelCase 字段形状）；
// 新版统计只读 SQLite。本文件在统计查询前把日志历史行幂等导入 SQLite，
// 新请求则由 logRequest 双写两处。requests.log 本身只读不写回。

type legacyFingerprint struct {
	size  int64
	mtime int64
}

var (
	legacyMu       sync.Mutex
	legacyImported = map[string]legacyFingerprint{}
)

// legacyLogPath resolves requests.log next to the active config file:
// PI_SWITCH_CONFIG dir > ~/.pi-switch/requests.log.
func legacyLogPath() string {
	return filepath.Join(filepath.Dir(configPath()), "requests.log")
}

// ensureLegacyImported imports missing requests.log lines into db.
// Guarded by file size+mtime fingerprint; on change only the appended
// tail is rescanned (the log is append-only; shrink triggers a full rescan).
// Safe to call per query. Missing file or bad lines never fatal.
func ensureLegacyImported(db *sql.DB) {
	path := legacyLogPath()
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return
	}
	fp := legacyFingerprint{size: fi.Size(), mtime: fi.ModTime().UnixNano()}
	legacyMu.Lock()
	if last, ok := legacyImported[path]; ok && last == fp {
		legacyMu.Unlock()
		return
	}
	offset := int64(0)
	if last, ok := legacyImported[path]; ok && last.size < fp.size {
		offset = last.size
	}
	legacyMu.Unlock()
	imported, skipped, completed := importLegacyLog(db, path, offset)
	if completed {
		legacyMu.Lock()
		legacyImported[path] = fp
		legacyMu.Unlock()
		if imported+skipped > 0 {
			log.Printf("legacy requests.log import: %d imported, %d skipped (%s)", imported, skipped, path)
		}
	}
}

// importLegacyLog scans path from offset and inserts absent rows.
// Returns imported/skipped counts; completed=false on read error
// (fingerprint must not advance, so the next call retries).
// The write side is wrapped in a single transaction for 23k-line
// imports so the daemon health window is not the bottleneck; when
// called in the background (ImportLegacyOnStartup) the listener is
// already up, but the transaction still avoids 23k autocommits.
func importLegacyLog(db *sql.DB, path string, offset int64) (imported, skipped int, completed bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			offset = 0
			if _, err := f.Seek(0, 0); err != nil {
				return 0, 0, false
			}
		}
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	completed = true
	tx, err := db.Begin()
	if err != nil {
		tx = nil
	}
	if tx != nil {
		defer func() {
			if completed {
				_ = tx.Commit()
			} else {
				_ = tx.Rollback()
			}
		}()
	}
	execDB := func(q string, args ...interface{}) (int64, error) {
		var res sql.Result
		var e error
		if tx != nil {
			res, e = tx.Exec(q, args...)
		} else {
			res, e = db.Exec(q, args...)
		}
		if e != nil {
			return 0, e
		}
		n, _ := res.RowsAffected()
		return n, nil
	}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var v map[string]interface{}
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			skipped++
			continue
		}
		ts := legacyStr(v, "ts")
		if ts == "" {
			skipped++
			continue
		}
		b, hasSuccess := v["ok"].(bool)
		if !hasSuccess {
			skipped++
			continue
		}
		provider := legacyStr(v, "provider")
		model := legacyStr(v, "model")
		pt := legacyInt(v, "promptTokens")
		ct := legacyInt(v, "completionTokens")
		cached := legacyInt(v, "cachedTokens")
		reasoning := legacyInt(v, "reasoningTokens")
		var costVal interface{}
		if f, ok := v["costTotal"].(float64); ok {
			costVal = f
		}
		convID := legacyStr(v, "conversationId")
		convName := legacyStr(v, "conversationName")
		succ := 0
		if b {
			succ = 1
		}
		n, err := execDB(`INSERT INTO requests(ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id,conversation_name,latency_ms)
			SELECT ?,?,?,?,?,?,?,?,?,?,?,? WHERE NOT EXISTS (
				SELECT 1 FROM requests WHERE ts=? AND provider=? AND model=? AND prompt_tokens=? AND completion_tokens=?
			)`, ts, provider, model, succ, pt, ct, cached, reasoning, costVal, convID, convName, nil,
			ts, provider, model, pt, ct)
		if err != nil {
			skipped++
			continue
		}
		if n == 1 {
			imported++
		} else {
			skipped++
		}
	}
	if err := sc.Err(); err != nil {
		completed = false
	}
	return imported, skipped, completed
}

func legacyStr(v map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if s, ok := v[k].(string); ok {
			return s
		}
	}
	return ""
}

func legacyInt(v map[string]interface{}, keys ...string) int64 {
	for _, k := range keys {
		if f, ok := v[k].(float64); ok {
			return int64(f)
		}
	}
	return 0
}

// insertLegacyRow maps one old-shape log line into requests.
// Returns (inserted, valid): valid=false for rows without the minimum
// usable fields (ts + ok); inserted=false for natural-key duplicates.
func insertLegacyRow(db *sql.DB, v map[string]interface{}) (inserted, valid bool) {
	ts := legacyStr(v, "ts")
	if ts == "" {
		return false, false
	}
	b, hasSuccess := v["ok"].(bool)
	success := b
	if !hasSuccess {
		return false, false
	}
	provider := legacyStr(v, "provider")
	model := legacyStr(v, "model")
	pt := legacyInt(v, "promptTokens")
	ct := legacyInt(v, "completionTokens")
	cached := legacyInt(v, "cachedTokens")
	reasoning := legacyInt(v, "reasoningTokens")
	var costVal interface{}
	if f, ok := v["costTotal"].(float64); ok {
		costVal = f
	}
	convID := legacyStr(v, "conversationId")
	convName := legacyStr(v, "conversationName")
	succ := 0
	if success {
		succ = 1
	}
	res, err := db.Exec(`INSERT INTO requests(ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id,conversation_name,latency_ms)
		SELECT ?,?,?,?,?,?,?,?,?,?,?,? WHERE NOT EXISTS (
			SELECT 1 FROM requests WHERE ts=? AND provider=? AND model=? AND prompt_tokens=? AND completion_tokens=?
		)`, ts, provider, model, succ, pt, ct, cached, reasoning, costVal, convID, convName, nil,
		ts, provider, model, pt, ct)
	if err != nil {
		return false, true
	}
	n, _ := res.RowsAffected()
	return n == 1, true
}

// appendLegacyLog appends one old-shape JSON line to requests.log.
// Best-effort: failures are swallowed so logging never breaks proxying.
func appendLegacyLog(entry map[string]interface{}) {
	path := legacyLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
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

// ImportLegacyOnStartup imports requests.log history once at process start.
// Runs in a background goroutine so it never blocks the HTTP listener
// (23k lines with per-row INSERT would exceed the daemon health window).
// Failures are swallowed so startup never breaks.
func ImportLegacyOnStartup() {
	go func() {
		db, err := store.GetDB()
		if err != nil {
			return
		}
		ensureLegacyImported(db)
	}()
}
