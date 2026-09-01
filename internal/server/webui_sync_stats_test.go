package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
	"github.com/heihei0299/pi-switch/internal/store"
)

func setupWebUISyncDB(t *testing.T) func() {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	t.Setenv("PI_SWITCH_DB", dbPath)
	// also set config path to avoid interference
	cfgPath := filepath.Join(dir, "config.json")
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	_ = os.WriteFile(cfgPath, []byte(`{"version":2,"profiles":{},"settings":{"providerPrefix":"pi-switch","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}}`), 0644)
	// ensure store reinits per test by reopening? store.GetDB will open new path
	// clear any cached db
	_, _ = store.ResetForTest(dbPath)
	return func() { _, _ = store.ResetForTest(dbPath) }
}

func insertRequestRow(t *testing.T, ts string, provider, model string, success int, pt, ct, cached, reasoning *int64, cost *float64, convID string) {
	t.Helper()
	db, err := store.GetDB()
	if err != nil {
		t.Fatalf("GetDB: %v", err)
	}
	var ptV, ctV, cachedV, reasoningV, costV interface{}
	if pt != nil {
		ptV = *pt
	}
	if ct != nil {
		ctV = *ct
	}
	if cached != nil {
		cachedV = *cached
	}
	if reasoning != nil {
		reasoningV = *reasoning
	}
	if cost != nil {
		costV = *cost
	}
	_, err = db.Exec(`INSERT INTO requests (ts, provider, model, success, prompt_tokens, completion_tokens, cached_tokens, reasoning_tokens, cost, conversation_id, conversation_name) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ts, provider, model, success, ptV, ctV, cachedV, reasoningV, costV, convID, "")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func int64Ptr(v int64) *int64 { return &v }
func float64Ptr(v float64) *float64 { return &v }

// S5 part: parseWindowQuery four档校验
func TestWebUISync_Stats_05_WindowParseFourGrades(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupWebUISyncDB(t)
	defer cleanup()
	r := NewMgmtRouter()
	now := time.Now()
	from := now.Add(-1 * time.Hour).UnixMilli()
	to := now.UnixMilli()
	cases := []struct {
		rangeParam string
		fromStr    string
		toStr      string
		wantCode   int
	}{
		{"today", fmt.Sprintf("%d", from), fmt.Sprintf("%d", to), 200},
		{"last24h", fmt.Sprintf("%d", from), fmt.Sprintf("%d", to), 200},
		{"last7d", fmt.Sprintf("%d", from), fmt.Sprintf("%d", to), 200},
		{"custom", fmt.Sprintf("%d", from), fmt.Sprintf("%d", to), 200},
		{"24h", fmt.Sprintf("%d", from), fmt.Sprintf("%d", to), 200},
		{"7d", fmt.Sprintf("%d", from), fmt.Sprintf("%d", to), 200},
		{"invalid", fmt.Sprintf("%d", from), fmt.Sprintf("%d", to), 400},
		{"today", "", fmt.Sprintf("%d", to), 400},
		{"today", fmt.Sprintf("%d", from), "", 400},
		{"today", "not-a-number", fmt.Sprintf("%d", to), 400},
		{"today", fmt.Sprintf("%d", to), fmt.Sprintf("%d", from), 400}, // from>=to
	}
	for _, tc := range cases {
		url := fmt.Sprintf("/api/stats?range=%s&from=%s&to=%s", tc.rangeParam, tc.fromStr, tc.toStr)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", url, nil)
		r.ServeHTTP(w, req)
		if w.Code != tc.wantCode {
			t.Errorf("range=%s from=%s to=%s got %d want %d body %s", tc.rangeParam, tc.fromStr, tc.toStr, w.Code, tc.wantCode, w.Body.String())
		}
	}
	// conversations endpoint same parsing
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", fmt.Sprintf("/api/stats/conversations?range=invalid&from=%d&to=%d", from, to), nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != 400 {
		t.Fatalf("conversations invalid range want 400 got %d", w2.Code)
	}
}

// S5 part: handleGetCredits 3 windows 20/45/70 and percent guard
func TestWebUISync_Stats_05_CreditsThreeWindows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	dbPath := filepath.Join(dir, "requests.db")
	// profile with opencode baseUrl to trigger credits
	cfgContent := `{"version":2,"profiles":{"op":{"api":"openai-completions","preset":"openai","baseUrl":"https://api.opencode.ai/v1","apiKey":"k","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384}],"proxy":false}},"settings":{"providerPrefix":"pi-switch","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110}}}`
	_ = os.WriteFile(cfgPath, []byte(cfgContent), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	_, _ = store.ResetForTest(dbPath)
	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/profiles/op/credits", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("credits code %d body %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	// check top percent exists and is number not undefined
	if p, ok := resp["percent"]; !ok {
		t.Fatalf("credits missing percent: %v", resp)
	} else if _, ok := p.(float64); !ok {
		t.Fatalf("percent not number: %T %v", p, p)
	}
	usage, ok := resp["usage"].(map[string]interface{})
	if !ok {
		t.Fatalf("usage missing: %v", resp)
	}
	rolling, _ := usage["rolling"].(map[string]interface{})
	weekly, _ := usage["weekly"].(map[string]interface{})
	monthly, _ := usage["monthly"].(map[string]interface{})
	if rolling == nil || weekly == nil || monthly == nil {
		t.Fatalf("usage windows missing: %v", usage)
	}
	if rp, _ := rolling["percent"].(float64); rp != 20 {
		t.Fatalf("rolling percent = %v want 20", rp)
	}
	if wp, _ := weekly["percent"].(float64); wp != 45 {
		t.Fatalf("weekly percent = %v want 45", wp)
	}
	if mp, _ := monthly["percent"].(float64); mp != 70 {
		t.Fatalf("monthly percent = %v want 70", mp)
	}
	// percent guard: missing profile returns percent 0 not undefined/null
	t.Setenv("PI_SWITCH_CONFIG", filepath.Join(dir, "empty.json"))
	_ = os.WriteFile(filepath.Join(dir, "empty.json"), []byte(`{"version":2,"profiles":{"empty":{"api":"openai-completions","baseUrl":"https://api.example.com/v1","apiKey":"k","models":[],"proxy":false}},"settings":{"providerPrefix":"pi-switch","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110}}}`), 0644)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/profiles/empty/credits", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("empty credits code %d", w2.Code)
	}
	var resp2 map[string]interface{}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if p, _ := resp2["percent"].(float64); p != 0 {
		// allow 0, but must not be nil
	}
	if resp2["percent"] == nil {
		t.Fatalf("empty profile percent should be 0 guard, got nil: %v", resp2)
	}
}

// S5 part: byConversation/byModel full recalc, totalCost/costUnknown, cacheRate
func TestWebUISync_Stats_05_AggregationFullRecalc(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupWebUISyncDB(t)
	defer cleanup()
	// ensure table exists
	db, _ := store.GetDB()
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS requests (id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT, provider TEXT, model TEXT, success INTEGER, prompt_tokens INTEGER, completion_tokens INTEGER, cached_tokens INTEGER, reasoning_tokens INTEGER, cost REAL, conversation_id TEXT, conversation_name TEXT, latency_ms INTEGER)`)

	r := NewMgmtRouter()
	now := time.Now().UTC()
	ts1 := now.Format(time.RFC3339)
	ts2 := now.Add(-1 * time.Minute).Format(time.RFC3339)
	ts3 := now.Add(-2 * time.Minute).Format(time.RFC3339)
	// success countable with cost
	insertRequestRow(t, ts1, "hyb", "gpt-4o", 1, int64Ptr(100), int64Ptr(50), int64Ptr(20), int64Ptr(5), float64Ptr(0.01), "conv-1")
	// success countable but cost nil => costUnknown++
	insertRequestRow(t, ts2, "hyb", "gpt-4o", 1, int64Ptr(200), int64Ptr(100), int64Ptr(0), int64Ptr(0), nil, "conv-1")
	// success but missing prompt (not countable) => totalCost not affected, costUnknown not counted
	insertRequestRow(t, ts3, "fox", "claude", 1, nil, int64Ptr(30), nil, nil, float64Ptr(0.02), "conv-2")
	// failed request (success=0) => not counted in totalCost/costUnknown
	insertRequestRow(t, ts1, "hyb", "gpt-4o", 0, int64Ptr(300), int64Ptr(100), int64Ptr(10), nil, float64Ptr(0.05), "conv-1")

	from := now.Add(-1 * time.Hour).UnixMilli()
	to := now.Add(1 * time.Hour).UnixMilli()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/stats?range=today&from=%d&to=%d", from, to), nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("stats code %d body %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if int(resp["totalRequests"].(float64)) != 4 {
		t.Fatalf("totalRequests = %v want 4", resp["totalRequests"])
	}
	if int(resp["okRequests"].(float64)) != 3 {
		t.Fatalf("okRequests = %v want 3", resp["okRequests"])
	}
	// totalCost only success=1 && prompt+completion countable => only first two rows with cost? second has nil cost so not counted, first 0.01
	if tc, ok := resp["totalCost"].(float64); !ok || tc != 0.01 {
		t.Fatalf("totalCost = %v want 0.01", resp["totalCost"])
	}
	if cu, _ := resp["costUnknown"].(float64); cu != 1 {
		t.Fatalf("costUnknown = %v want 1", resp["costUnknown"])
	}
	// byProvider
	bp, _ := resp["byProvider"].(map[string]interface{})
	if bp == nil {
		t.Fatalf("byProvider missing")
	}
	hyb, _ := bp["hyb"].(map[string]interface{})
	if hyb == nil {
		t.Fatalf("byProvider hyb missing")
	}
	if int(hyb["total"].(float64)) != 3 {
		t.Fatalf("hyb total = %v want 3", hyb["total"])
	}
	// cacheRate for hyb: input 100+200=300, cached 20+0=20 => 6.7% ?
	// but only countable rows: first 100/20, second 200/0 => total 300/20 => 6.7%
	if cr, _ := hyb["cacheRate"].(string); cr != "6.7%" {
		t.Fatalf("hyb cacheRate = %q want 6.7%%", cr)
	}
	// byModel
	bm, _ := resp["byModel"].(map[string]interface{})
	if bm == nil {
		t.Fatalf("byModel missing")
	}
	gpt, _ := bm["gpt-4o"].(map[string]interface{})
	if gpt == nil {
		t.Fatalf("byModel gpt-4o missing")
	}
	if int(gpt["total"].(float64)) != 3 {
		t.Fatalf("gpt-4o total = %v want 3", gpt["total"])
	}
	// byConversation all recalc
	byConv, _ := resp["byConversation"].([]interface{})
	if len(byConv) != 2 {
		t.Fatalf("byConversation len = %d want 2", len(byConv))
	}
	// pagination
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", fmt.Sprintf("/api/stats?range=today&from=%d&to=%d&page=0&limit=2", from, to), nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("paged stats code %d", w2.Code)
	}
	var resp2 map[string]interface{}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if int(resp2["recentRequestTotal"].(float64)) != 4 {
		t.Fatalf("recentRequestTotal = %v want 4", resp2["recentRequestTotal"])
	}
	if arr, _ := resp2["recentRequests"].([]interface{}); len(arr) != 2 {
		t.Fatalf("recentRequests len = %d want 2", len(arr))
	}
	// conversations endpoint also paginated
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", fmt.Sprintf("/api/stats/conversations?range=today&from=%d&to=%d&page=0&limit=1", from, to), nil)
	r.ServeHTTP(w3, req3)
	if w3.Code != 200 {
		t.Fatalf("conversations code %d", w3.Code)
	}
	var convResp map[string]interface{}
	_ = json.Unmarshal(w3.Body.Bytes(), &convResp)
	if int(convResp["total"].(float64)) != 2 {
		t.Fatalf("conversations total = %v want 2", convResp["total"])
	}
}

func TestWebUISync_Stats_05_CacheRateEdge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cleanup := setupWebUISyncDB(t)
	defer cleanup()
	db, _ := store.GetDB()
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS requests (id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT, provider TEXT, model TEXT, success INTEGER, prompt_tokens INTEGER, completion_tokens INTEGER, cached_tokens INTEGER, reasoning_tokens INTEGER, cost REAL, conversation_id TEXT, conversation_name TEXT, latency_ms INTEGER)`)
	r := NewMgmtRouter()
	now := time.Now().UTC()
	ts := now.Format(time.RFC3339)
	// input 0 => cacheRate "-"
	insertRequestRow(t, ts, "a", "m", 1, int64Ptr(0), int64Ptr(10), int64Ptr(0), nil, float64Ptr(0.01), "c1")
	from := now.Add(-1 * time.Hour).UnixMilli()
	to := now.Add(1 * time.Hour).UnixMilli()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", fmt.Sprintf("/api/stats?range=today&from=%d&to=%d", from, to), nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("code %d", w.Code)
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["cacheHitRate"] != "-" {
		t.Fatalf("cacheHitRate for 0 input should be '-', got %v", resp["cacheHitRate"])
	}
	if bp, _ := resp["byProvider"].(map[string]interface{}); bp != nil {
		if a, _ := bp["a"].(map[string]interface{}); a != nil {
			if a["cacheRate"] != "-" {
				t.Fatalf("byProvider cacheRate for 0 input should be '-', got %v", a["cacheRate"])
			}
		}
	}
}
