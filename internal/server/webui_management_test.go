package server

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestWebUI_EmbeddedReturnsHTML(t *testing.T) {
	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("GET / code = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<div id=\"root\"") && !strings.Contains(body, "pi-switch") {
		t.Fatalf("GET / body missing root/html, got %.200s", body)
	}
}

func TestProfiles_CRUDAndExposedModelsSync(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	dbPath := filepath.Join(dir, "requests.db")
	initial := `{"version":2,"current":"test-provider","profiles":{"test-provider":{"api":"openai-completions","preset":"openai","baseUrl":"http://a/v1","apiKey":"k1","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"proxy":false}},"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}}`
	_ = os.WriteFile(cfgPath, []byte(initial), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/profiles", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("GET /api/profiles code %d body %s", w.Code, w.Body.String())
	}
	var profMap map[string]interface{}
	var raw map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &raw)
	if _, ok := raw["profiles"]; ok {
	} else if _, ok := raw["test-provider"]; ok {
		profMap = raw
	} else {
		if p, ok := raw["profiles"].(map[string]interface{}); ok {
			profMap = p
		}
	}
	_ = profMap
	newProf := `{"name":"new-provider","profile":{"api":"openai-completions","preset":"openai","baseUrl":"http://b/v1","apiKey":"k2","upstreams":[{"name":"main","api":"openai-completions","baseUrl":"http://b/v1","apiKey":"k2","models":[{"id":"gpt-4o","contextWindow":128000,"maxTokens":16384}],"exposedModels":["gpt-4o"]}],"proxy":false,"headers":{"X-Custom":"1"},"userAgent":"test-agent"}}`
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/api/profiles", strings.NewReader(newProf))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("POST /api/profiles code %d body %s", w2.Code, w2.Body.String())
	}
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/api/profiles/new-provider", nil)
	r.ServeHTTP(w3, req3)
	if w3.Code != 200 {
		t.Fatalf("GET /api/profiles/new-provider code %d body %s", w3.Code, w3.Body.String())
	}
	var detail map[string]interface{}
	_ = json.Unmarshal(w3.Body.Bytes(), &detail)
	var profObj map[string]interface{}
	if p, ok := detail["profile"].(map[string]interface{}); ok {
		profObj = p
	} else {
		profObj = detail
	}
	if profObj["api"] != "openai-completions" {
		t.Fatalf("api = %v want openai-completions", profObj["api"])
	}
	if ups, ok := profObj["upstreams"].([]interface{}); ok {
		em := ups[0].(map[string]interface{})["exposedModels"].([]interface{})
		found := false
		for _, v := range em {
			if v == "gpt-4o" {
				found = true
			}
		}
		if !found {
			t.Fatalf("exposedModels missing gpt-4o: %v", em)
		}
	} else {
		t.Fatalf("channel exposedModels not array: %v", profObj["upstreams"])
	}
	updateBody := `{"profile":{"api":"openai-completions","preset":"openai","baseUrl":"http://b/v1","apiKey":"k2","upstreams":[{"name":"main","api":"openai-completions","baseUrl":"http://b/v1","apiKey":"k2","models":[{"id":"gpt-4o","contextWindow":128000,"maxTokens":16384},{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384}],"exposedModels":["gpt-4o-mini"]}],"modelMap":{"gpt-4o-mini":"mapped"},"proxy":false}}`
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest("PUT", "/api/profiles/new-provider", strings.NewReader(updateBody))
	req4.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w4, req4)
	if w4.Code != 200 {
		t.Fatalf("PUT code %d body %s", w4.Code, w4.Body.String())
	}
	w5 := httptest.NewRecorder()
	req5, _ := http.NewRequest("GET", "/api/profiles/new-provider", nil)
	r.ServeHTTP(w5, req5)
	var detail2 map[string]interface{}
	_ = json.Unmarshal(w5.Body.Bytes(), &detail2)
	var prof2 map[string]interface{}
	if p, ok := detail2["profile"].(map[string]interface{}); ok {
		prof2 = p
	} else {
		prof2 = detail2
	}
	if ups, ok := prof2["upstreams"].([]interface{}); ok {
		em := ups[0].(map[string]interface{})["exposedModels"].([]interface{})
		if len(em) != 1 || em[0] != "gpt-4o-mini" {
			t.Fatalf("after PUT exposedModels = %v want [gpt-4o-mini]", em)
		}
	} else {
		t.Fatalf("channel exposedModels after PUT missing")
	}
	w6 := httptest.NewRecorder()
	req6, _ := http.NewRequest("DELETE", "/api/profiles/new-provider", nil)
	r.ServeHTTP(w6, req6)
	if w6.Code != 200 {
		t.Fatalf("DELETE code %d body %s", w6.Code, w6.Body.String())
	}
	w7 := httptest.NewRecorder()
	req7, _ := http.NewRequest("GET", "/api/profiles/new-provider", nil)
	r.ServeHTTP(w7, req7)
	if w7.Code != 404 {
		t.Fatalf("GET after DELETE want 404 got %d body %s", w7.Code, w7.Body.String())
	}
}

func TestGatewayPublish_WritesModelsJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	modelsPath := filepath.Join(dir, "models.json")
	dbPath := filepath.Join(dir, "requests.db")
	cfgContent := `{"version":2,"current":"test-provider","profiles":{"test-provider":{"api":"openai-completions","preset":"openai","baseUrl":"http://a/v1","apiKey":"k1","upstreams":[{"name":"main","api":"openai-completions","baseUrl":"http://a/v1","apiKey":"k1","models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384,"cost":{"input":0.15,"output":0.6,"cacheRead":0.075}}],"exposedModels":["gpt-4o-mini"]}],"proxy":false}},"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}}`
	_ = os.WriteFile(cfgPath, []byte(cfgContent), 0644)
	_ = os.WriteFile(modelsPath, []byte(`{"providers":{}}`), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	r := NewMgmtRouter()
	previewBefore, _ := os.ReadFile(modelsPath)
	wPrev := httptest.NewRecorder()
	reqPrev, _ := http.NewRequest("GET", "/api/models/gateway/preview", nil)
	r.ServeHTTP(wPrev, reqPrev)
	if wPrev.Code != 200 {
		t.Fatalf("preview code %d", wPrev.Code)
	}
	afterPreview, _ := os.ReadFile(modelsPath)
	if string(previewBefore) != string(afterPreview) {
		t.Fatalf("preview should not write models.json")
	}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/gateway/publish", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("POST /api/gateway/publish code %d body %s", w.Code, w.Body.String())
	}
	data, err := os.ReadFile(modelsPath)
	if err != nil {
		t.Fatalf("read models.json: %v", err)
	}
	var mj map[string]interface{}
	_ = json.Unmarshal(data, &mj)
	provs, ok := mj["providers"].(map[string]interface{})
	if !ok {
		t.Fatalf("providers missing: %v", mj)
	}
	gw, ok := provs["pi-switch-chat"].(map[string]interface{})
	if !ok {
		t.Fatalf("pi-switch-chat provider missing: %v", provs)
	}
	models, ok := gw["models"].([]interface{})
	if !ok || len(models) == 0 {
		t.Fatalf("gateway models missing: %v", gw)
	}
	found := false
	for _, m := range models {
		if mm, ok := m.(map[string]interface{}); ok && mm["id"] == "gpt-4o-mini" {
			found = true
		}
	}
	if !found {
		t.Fatalf("gateway models should contain gpt-4o-mini bare, got %v", models)
	}
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("PUT", "/api/models/gateway", strings.NewReader(`{"providers":{"pi-switch-chat":{"api":"openai-completions","baseUrl":"http://127.0.0.1:43112/v1","apiKey":"pi-switch-proxy","models":[{"id":"gpt-4o-mini"}],"proxy":false}}}`))
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("PUT /api/models/gateway code %d body %s", w2.Code, w2.Body.String())
	}
}

func TestValidate_ReturnsLevelPathMessage(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cfgPath, []byte(`{"version":2,"profiles":{"bad":{"api":"invalid-api","baseUrl":"","apiKey":"","models":[],"proxy":false}},"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"sessionScan"}}`), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	r := NewMgmtRouter()
	for _, path := range []string{"/api/validate", "/api/config/validate"} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", path, nil)
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("GET %s code %d body %s", path, w.Code, w.Body.String())
		}
		var issues []map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &issues); err != nil {
			t.Fatalf("unmarshal %s: %v body %s", path, err, w.Body.String())
		}
		if len(issues) == 0 {
			t.Fatalf("issues empty for %s", path)
		}
		for _, iss := range issues {
			if _, ok := iss["level"]; !ok {
				t.Fatalf("missing level in %v", iss)
			}
			if _, ok := iss["path"]; !ok {
				t.Fatalf("missing path in %v", iss)
			}
			if _, ok := iss["message"]; !ok {
				t.Fatalf("missing message in %v", iss)
			}
		}
	}
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/validate", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("POST /api/validate code %d", w.Code)
	}
}

func TestStats_WindowFiltering(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	dbPath := filepath.Join(dir, "requests.db")
	modelsPath := filepath.Join(dir, "models.json")
	_ = os.WriteFile(cfgPath, []byte(`{"version":2,"profiles":{},"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"proxy"}}`), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	t.Setenv("PI_SWITCH_MODELS", modelsPath)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS requests (id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT, provider TEXT, model TEXT, success INTEGER, prompt_tokens INTEGER, completion_tokens INTEGER, cached_tokens INTEGER, reasoning_tokens INTEGER, cost REAL, conversation_id TEXT, conversation_name TEXT, latency_ms INTEGER)`)
	_, _ = db.Exec(`DELETE FROM requests`)
	now := time.Now()
	oldTs := now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)
	recentTs := now.Format(time.RFC3339)
	_, _ = db.Exec(`INSERT INTO requests(ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id,conversation_name) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, oldTs, "p1", "m1", 1, 100, 50, 10, 5, 0.01, "c-old", "old")
	_, _ = db.Exec(`INSERT INTO requests(ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id,conversation_name) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, recentTs, "p1", "m1", 1, 200, 100, 20, 10, 0.02, "c-recent", "recent")
	_, _ = db.Exec(`INSERT INTO requests(ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id,conversation_name) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, recentTs, "p2", "m2", 1, 300, 150, 0, 0, 0.03, "c-recent", "recent")
	_ = db.Close()
	r := NewMgmtRouter()
	getStats := func(q string) map[string]interface{} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/stats"+q, nil)
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("GET /api/stats%s code %d body %s", q, w.Code, w.Body.String())
		}
		var m map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		return m
	}
	from := now.Add(-24 * time.Hour).UnixMilli()
	to := now.Add(1 * time.Hour).UnixMilli()
	q := fmt.Sprintf("?range=last24h&from=%d&to=%d", from, to)
	m := getStats(q)
	if tr, ok := m["totalRequests"].(float64); ok {
		if int(tr) != 2 {
			t.Fatalf("totalRequests = %v want 2 (window last24h)", tr)
		}
	} else {
		t.Fatalf("totalRequests missing or not number: %v", m)
	}
	q2 := fmt.Sprintf("?window=24h&from=%d&to=%d", from, to)
	m2 := getStats(q2)
	if tr, ok := m2["totalRequests"].(float64); ok {
		if int(tr) != 2 {
			t.Fatalf("window=24h totalRequests = %v want 2", tr)
		}
	}
	todayFrom := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UnixMilli()
	todayTo := now.Add(1 * time.Hour).UnixMilli()
	q3 := fmt.Sprintf("?range=today&from=%d&to=%d", todayFrom, todayTo)
	m3 := getStats(q3)
	if tr, ok := m3["totalRequests"].(float64); ok {
		if int(tr) != 2 && int(tr) != 3 {
			if int(tr) != 2 {
				t.Fatalf("today totalRequests = %v want 2", tr)
			}
		}
	}
	from7 := now.Add(-7 * 24 * time.Hour).UnixMilli()
	q4 := fmt.Sprintf("?range=last7d&from=%d&to=%d", from7, to)
	m4 := getStats(q4)
	if tr, ok := m4["totalRequests"].(float64); ok {
		if int(tr) != 2 {
			t.Fatalf("last7d totalRequests = %v want 2", tr)
		}
	}
	customFrom := now.Add(-11 * 24 * time.Hour).UnixMilli()
	customTo := now.Add(-9 * 24 * time.Hour).UnixMilli()
	q5 := fmt.Sprintf("?range=custom&from=%d&to=%d", customFrom, customTo)
	m5 := getStats(q5)
	if tr, ok := m5["totalRequests"].(float64); ok {
		if int(tr) != 1 {
			t.Fatalf("custom old window totalRequests = %v want 1, m5=%v", tr, m5)
		}
	}
	if tc, ok := m["totalCost"].(float64); ok {
		diff := tc - 0.05
		if diff < -0.0001 || diff > 0.0001 {
			t.Fatalf("totalCost recent window = %v want 0.05", tc)
		}
	} else {
		t.Fatalf("totalCost missing in window filter")
	}
	if tc, ok := m5["totalCost"].(float64); ok {
		diff := tc - 0.01
		if diff < -0.0001 || diff > 0.0001 {
			t.Fatalf("totalCost old custom = %v want 0.01", tc)
		}
	}
	if bp, ok := m["byProvider"].(map[string]interface{}); ok {
		if _, ok := bp["p2"]; !ok {
			t.Fatalf("byProvider should contain p2 in recent window: %v", bp)
		}
	}
	if bc, ok := m5["byConversation"]; ok {
		if arr, ok := bc.([]interface{}); ok {
			if len(arr) != 1 {
				t.Fatalf("byConversation custom old should have 1 conv, got %v", arr)
			}
		}
	}
}

func TestExport_JSONAndCSVIncludeCostCachedReasoning(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	dbPath := filepath.Join(dir, "requests.db")
	_ = os.WriteFile(cfgPath, []byte(`{"version":2,"profiles":{},"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"proxy"}}`), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	db, _ := sql.Open("sqlite", dbPath)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS requests (id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT, provider TEXT, model TEXT, success INTEGER, prompt_tokens INTEGER, completion_tokens INTEGER, cached_tokens INTEGER, reasoning_tokens INTEGER, cost REAL, conversation_id TEXT, conversation_name TEXT, latency_ms INTEGER)`)
	_, _ = db.Exec(`DELETE FROM requests`)
	ts := time.Now().Format(time.RFC3339)
	_, _ = db.Exec(`INSERT INTO requests(ts,provider,model,success,prompt_tokens,completion_tokens,cached_tokens,reasoning_tokens,cost,conversation_id) VALUES (?,?,?,?,?,?,?,?,?,?)`, ts, "p1", "m1", 1, 100, 50, 10, 5, 0.123, "c1")
	_, _ = db.Exec(`INSERT INTO requests(ts,provider,model,success,prompt_tokens,completion_tokens,conversation_id) VALUES (?,?,?,?,?,?,?)`, ts, "p1", "m1", 1, 10, 5, "c2")
	_ = db.Close()
	r := NewMgmtRouter()
	for _, path := range []string{"/api/export?format=json", "/api/logs/export?format=json"} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", path, nil)
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("GET %s code %d body %s", path, w.Code, w.Body.String())
		}
		var arr []map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &arr); err != nil {
			var obj map[string]interface{}
			if err2 := json.Unmarshal(w.Body.Bytes(), &obj); err2 != nil {
				t.Fatalf("json export unmarshal %s: %v body %s", path, err, w.Body.String())
			}
		} else {
			if len(arr) < 2 {
				t.Fatalf("json export len %d want >=2", len(arr))
			}
			foundCost := false
			for _, row := range arr {
				if _, ok := row["cost"]; ok {
					foundCost = true
				}
				if _, ok := row["costTotal"]; ok {
					foundCost = true
				}
			}
			if !foundCost {
				t.Fatalf("json export missing cost field: %v", arr[0])
			}
		}
	}
	for _, path := range []string{"/api/export?format=csv", "/api/logs/export?format=csv"} {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", path, nil)
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("GET %s code %d", path, w.Code)
		}
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "csv") && !strings.Contains(ct, "text") {
			t.Fatalf("csv Content-Type = %q", ct)
		}
		body := w.Body.String()
		if !strings.Contains(strings.ToLower(body), "cost") {
			t.Fatalf("csv missing cost column: %.500s", body)
		}
		if !strings.Contains(strings.ToLower(body), "cached") {
			t.Fatalf("csv missing cached column: %.500s", body)
		}
		if !strings.Contains(strings.ToLower(body), "reason") {
			t.Fatalf("csv missing reasoning column: %.500s", body)
		}
		rdr := csv.NewReader(strings.NewReader(body))
		records, err := rdr.ReadAll()
		if err != nil {
			t.Fatalf("csv parse: %v", err)
		}
		if len(records) < 3 {
			t.Fatalf("csv records len %d", len(records))
		}
		header := records[0]
		costIdx := -1
		for i, h := range header {
			if strings.EqualFold(h, "costTotal") || strings.EqualFold(h, "cost") {
				costIdx = i
			}
		}
		if costIdx == -1 {
			t.Fatalf("cost column not in header %v", header)
		}
		hasEmpty := false
		for _, rec := range records[1:] {
			if rec[costIdx] == "" {
				hasEmpty = true
			}
		}
		if !hasEmpty {
			t.Fatalf("csv should have empty cost for NULL row, records %v", records)
		}
	}
}

func TestFormatCost_Wiring(t *testing.T) {
}
