package server

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/bootstrap"
	"github.com/heihei0299/pi-switch/internal/store"
	"github.com/heihei0299/pi-switch/internal/usecase/analytics"
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

func statsWindowFor(c *gin.Context) (*analytics.Window, error) {
	rangeParam := c.Query("range")
	if rangeParam == "" {
		rangeParam = c.Query("window")
	}
	w, err := parseWindowQuery(rangeParam, c.Query("from"), c.Query("to"))
	if err != nil || w == nil {
		return nil, err
	}
	return &analytics.Window{From: w.from, To: w.to}, nil
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
	service, err := bootstrap.NewManagementAnalyticsService(db)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	response, err := service.StatsContext(c.Request.Context(), window, page, limit)
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
	service, err := bootstrap.NewManagementAnalyticsService(db)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	conversations, total, err := service.ConversationsContext(c.Request.Context(), window, page, limit)
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
	service, err := bootstrap.NewManagementAnalyticsService(db)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	requests, total, err := service.ConversationRequestsContext(c.Request.Context(), id, page, limit)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"requests": requests, "total": total})
	return
}

func handleLogsExport(c *gin.Context) {
	format := c.DefaultQuery("format", "json")
	if format != "csv" {
		format = "json"
	}
	db, err := store.GetDB()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if format == "csv" {
		c.Header("Content-Type", "text/csv")
		c.Header("Content-Disposition", `attachment; filename="pi-switch-logs.csv"`)
	} else {
		c.Header("Content-Type", "application/json")
		c.Header("Content-Disposition", `attachment; filename="pi-switch-logs.json"`)
	}
	if err := bootstrap.NewSummaryAnalyticsService(db).ExportRequestsContext(c.Request.Context(), c.Writer, format); err != nil && !c.Writer.Written() {
		c.JSON(500, gin.H{"error": err.Error()})
	}
}
