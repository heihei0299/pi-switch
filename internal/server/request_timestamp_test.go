package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	statsservice "github.com/heihei0299/pi-switch/internal/stats"
	"github.com/heihei0299/pi-switch/internal/store"
)

func TestLogRequestPreservesSubsecondTimestampForCurrentStatsWindow(t *testing.T) {
	writeLegacyTestEnv(t, "")
	t.Cleanup(store.Close)

	logRequest("provider", "model", true, 10, 5, 0, 0, nil, "", "", 12, 200, "", "")

	db, err := store.GetDB()
	if err != nil {
		t.Fatalf("get request db: %v", err)
	}
	var rawTS string
	if err := db.QueryRow(`SELECT ts FROM requests ORDER BY id DESC LIMIT 1`).Scan(&rawTS); err != nil {
		t.Fatalf("read request timestamp: %v", err)
	}
	parsed, err := time.Parse(time.RFC3339Nano, rawTS)
	if err != nil {
		t.Fatalf("parse request timestamp %q: %v", rawTS, err)
	}
	if parsed.Nanosecond() == 0 {
		t.Fatalf("request timestamp lost subsecond precision: %q", rawTS)
	}
	service, err := newStatsService(db)
	if err != nil {
		t.Fatalf("stats service: %v", err)
	}
	window := &statsservice.Window{
		From: parsed.Add(-time.Second).UnixMilli(),
		To:   parsed.UnixMilli() + 1,
	}
	response, err := service.Stats(window, 0, 50)
	if err != nil {
		t.Fatalf("stats query: %v", err)
	}
	if response.TotalRequests != 1 {
		t.Fatalf("current stats window totalRequests = %d, want 1", response.TotalRequests)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/stats?range=last24h&from=%d&to=%d", window.From, window.To), nil)
	NewMgmtRouter().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("stats HTTP status = %d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		TotalRequests int `json:"totalRequests"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalRequests != 1 {
		t.Fatalf("stats HTTP totalRequests = %d, want 1", body.TotalRequests)
	}
}
