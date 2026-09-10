package server

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/conversation"
	statsservice "github.com/heihei0299/pi-switch/internal/stats"
	"github.com/heihei0299/pi-switch/internal/store"
)

type window struct{ from, to int64 }

// --- stats helpers ---
func normalizeRange(s string) string {
	switch s {
	case "today":
		return "today"
	case "last24h", "24h":
		return "last24h"
	case "last7d", "7d":
		return "last7d"
	case "custom":
		return "custom"
	default:
		return ""
	}
}

func parseWindowQuery(rangeParam, fromStr, toStr string) (*window, error) {
	hasRange := strings.TrimSpace(rangeParam) != ""
	hasFrom := strings.TrimSpace(fromStr) != ""
	hasTo := strings.TrimSpace(toStr) != ""
	if !hasRange && !hasFrom && !hasTo {
		return nil, nil
	}
	if hasRange {
		norm := normalizeRange(rangeParam)
		if norm == "" {
			return nil, fmt.Errorf("invalid range: %s", rangeParam)
		}
		if !hasFrom || !hasTo {
			return nil, fmt.Errorf("window requires both from and to (epoch millis)")
		}
		fm, err := strconv.ParseInt(fromStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid from: %s", fromStr)
		}
		tm, err := strconv.ParseInt(toStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid to: %s", toStr)
		}
		if fm >= tm {
			return nil, fmt.Errorf("invalid window: from (%d) must be < to (%d)", fm, tm)
		}
		return &window{from: fm, to: tm}, nil
	}
	// no range but from/to present
	if hasFrom || hasTo {
		if !hasFrom || !hasTo {
			return nil, fmt.Errorf("window requires both from and to (epoch millis)")
		}
		fm, err := strconv.ParseInt(fromStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid from: %s", fromStr)
		}
		tm, err := strconv.ParseInt(toStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid to: %s", toStr)
		}
		if fm >= tm {
			return nil, fmt.Errorf("invalid window: from (%d) must be < to (%d)", fm, tm)
		}
		return &window{from: fm, to: tm}, nil
	}
	return nil, nil
}

func statsWindowFor(c *gin.Context) (*statsservice.Window, error) {
	rangeParam := c.Query("range")
	if rangeParam == "" {
		rangeParam = c.Query("window")
	}
	w, err := parseWindowQuery(rangeParam, c.Query("from"), c.Query("to"))
	if err != nil || w == nil {
		return nil, err
	}
	return &statsservice.Window{From: w.from, To: w.to}, nil
}

func statsPageLimit(c *gin.Context, max int) (page, limit int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "0"))
	limit, _ = strconv.Atoi(c.DefaultQuery("limit", "50"))
	if page < 0 {
		page = 0
	}
	if limit <= 0 {
		limit = 50
	}
	if max > 0 && limit > max {
		limit = max
	}
	return page, limit
}

func newStatsService(db *sql.DB) statsservice.Service {
	cfg, _, _ := config.LoadConfigAtPath(configPath())
	source := cfg.Settings.ConversationSource
	return statsservice.Service{
		DB:         db,
		Source:     conversation.Source(source),
		Candidates: conversationCandidates(sessionScanCandidates(source)),
	}
}

// --- stats handler with window filtering ---
func handleStats(c *gin.Context) {
	if c.Query("groupBy") == "conversation" {
		handleStatsConversations(c)
		return
	}
	window, err := statsWindowFor(c)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	page, limit := statsPageLimit(c, 500)
	db, err := store.GetDB()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	response, err := newStatsService(db).Stats(window, page, limit)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, response)
	return
}

func handleStatsConversations(c *gin.Context) {
	window, err := statsWindowFor(c)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	page, limit := statsPageLimit(c, 0)
	db, err := store.GetDB()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	conversations, total, err := newStatsService(db).Conversations(window, page, limit)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{
		"conversations":   conversations,
		"total":           total,
		"byConversation":  conversations,
		"by_conversation": conversations,
	})
	return
}

func handleConversationRequests(c *gin.Context) {
	id := c.Param("id")
	if strings.TrimSpace(id) == "" {
		c.JSON(400, gin.H{"error": "conversation id must not be empty"})
		return
	}
	page, limit := statsPageLimit(c, 0)
	db, err := store.GetDB()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	requests, total, err := newStatsService(db).ConversationRequests(id, page, limit)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"requests": requests, "total": total})
	return
}

func handleLogsExport(c *gin.Context) {
	format := c.DefaultQuery("format", "json")
	if format == "" {
		format = "json"
	}
	db, err := store.GetDB()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	rows, err := db.Query(`SELECT ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id,conversation_name,latency_ms FROM requests ORDER BY id ASC`)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	type rec struct {
		TS        sql.NullString
		Provider  sql.NullString
		Model     sql.NullString
		Success   sql.NullInt64
		PT        sql.NullInt64
		CT        sql.NullInt64
		Cached    sql.NullInt64
		Reasoning sql.NullInt64
		Cost      sql.NullFloat64
		ConvID    sql.NullString
		ConvName  sql.NullString
		Latency   sql.NullInt64
	}
	var recs []rec
	for rows.Next() {
		var r rec
		_ = rows.Scan(&r.TS, &r.Provider, &r.Model, &r.Success, &r.PT, &r.CT, &r.Cached, &r.Reasoning, &r.Cost, &r.ConvID, &r.ConvName, &r.Latency)
		recs = append(recs, r)
	}
	if format == "csv" {
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		header := []string{"timestamp", "ok", "provider", "model", "status", "latency_ms", "error", "retry", "skipped", "converted", "upstream_url", "promptTokens", "completionTokens", "cachedTokens", "reasoningTokens", "conversationId", "conversationName", "costTotal", "cost", "cached_tokens", "reasoning_tokens"}
		_ = w.Write(header)
		for _, r := range recs {
			okStr := ""
			if r.Success.Valid {
				if r.Success.Int64 == 1 {
					okStr = "true"
				} else {
					okStr = "false"
				}
			}
			status := ""
			if r.Success.Valid {
				if r.Success.Int64 == 1 {
					status = "200"
				} else {
					status = "500"
				}
			}
			lat := ""
			if r.Latency.Valid {
				lat = strconv.FormatInt(r.Latency.Int64, 10)
			}
			pt := ""
			if r.PT.Valid {
				pt = strconv.FormatInt(r.PT.Int64, 10)
			}
			ct := ""
			if r.CT.Valid {
				ct = strconv.FormatInt(r.CT.Int64, 10)
			}
			cached := ""
			if r.Cached.Valid {
				cached = strconv.FormatInt(r.Cached.Int64, 10)
			}
			reason := ""
			if r.Reasoning.Valid {
				reason = strconv.FormatInt(r.Reasoning.Int64, 10)
			}
			cost := ""
			if r.Cost.Valid {
				cost = strconv.FormatFloat(r.Cost.Float64, 'f', -1, 64)
			}
			ts := ""
			if r.TS.Valid {
				ts = r.TS.String
			}
			prov := ""
			if r.Provider.Valid {
				prov = r.Provider.String
			}
			mod := ""
			if r.Model.Valid {
				mod = r.Model.String
			}
			conv := ""
			if r.ConvID.Valid {
				conv = r.ConvID.String
			}
			convName := ""
			if r.ConvName.Valid {
				convName = r.ConvName.String
			}
			row := []string{ts, okStr, prov, mod, status, lat, "", "", "", "", "", pt, ct, cached, reason, conv, convName, cost, cost, cached, reason}
			_ = w.Write(row)
		}
		w.Flush()
		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", `attachment; filename="pi-switch-logs.csv"`)
		c.String(200, buf.String())
		return
	}
	// json
	var out []map[string]interface{}
	for _, r := range recs {
		m := map[string]interface{}{
			"ts": nil, "provider": nil, "model": nil, "success": nil, "ok": nil,
			"prompt_tokens": nil, "completion_tokens": nil, "cached_tokens": nil, "reasoning_tokens": nil,
			"cachedTokens": nil, "reasoningTokens": nil, "promptTokens": nil, "completionTokens": nil,
			"cost": nil, "costTotal": nil, "conversation_id": nil, "conversationId": nil, "conversation_name": nil, "conversationName": nil,
			"latency_ms": nil, "status": nil,
		}
		if r.TS.Valid {
			m["ts"] = r.TS.String
			m["timestamp"] = r.TS.String
		}
		if r.Provider.Valid {
			m["provider"] = r.Provider.String
		}
		if r.Model.Valid {
			m["model"] = r.Model.String
		}
		if r.Success.Valid {
			m["success"] = r.Success.Int64 == 1
			m["ok"] = r.Success.Int64 == 1
			if r.Success.Int64 == 1 {
				m["status"] = 200
			} else {
				m["status"] = 500
			}
		}
		if r.PT.Valid {
			m["prompt_tokens"] = r.PT.Int64
			m["promptTokens"] = r.PT.Int64
		}
		if r.CT.Valid {
			m["completion_tokens"] = r.CT.Int64
			m["completionTokens"] = r.CT.Int64
		}
		if r.Cached.Valid {
			m["cached_tokens"] = r.Cached.Int64
			m["cachedTokens"] = r.Cached.Int64
		}
		if r.Reasoning.Valid {
			m["reasoning_tokens"] = r.Reasoning.Int64
			m["reasoningTokens"] = r.Reasoning.Int64
		}
		if r.Cost.Valid {
			m["cost"] = r.Cost.Float64
			m["costTotal"] = r.Cost.Float64
		}
		if r.ConvID.Valid {
			m["conversation_id"] = r.ConvID.String
			m["conversationId"] = r.ConvID.String
		}
		if r.ConvName.Valid {
			m["conversation_name"] = r.ConvName.String
			m["conversationName"] = r.ConvName.String
		}
		if r.Latency.Valid {
			m["latency_ms"] = r.Latency.Int64
		}
		out = append(out, m)
	}
	if out == nil {
		out = []map[string]interface{}{}
	}
	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", `attachment; filename="pi-switch-logs.json"`)
	c.JSON(200, out)
}
