package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/store"
)

func writeLegacyTestEnv(t *testing.T, logContent string) (dir string) {
	t.Helper()
	dir = t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	cfgPath := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cfgPath, []byte(`{"version":2,"profiles":{},"settings":{"providerPrefix":"pi-switch","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"proxy"}}`), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	if _, err := store.ResetForTest(dbPath); err != nil {
		t.Fatalf("reset db: %v", err)
	}
	if logContent != "" {
		if err := os.WriteFile(filepath.Join(dir, "requests.log"), []byte(logContent), 0644); err != nil {
			t.Fatalf("write legacy log: %v", err)
		}
	}
	return dir
}

func getStatsMap(t *testing.T, r http.Handler) map[string]interface{} {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stats", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("GET /api/stats code = %d, body %s", w.Code, w.Body.String())
	}
	var m map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	return m
}

// T1: 旧版本请求日志行出现在新版统计中（坏行跳过，缺消费记 unknown）
func TestLegacyLog_ImportedIntoStats(t *testing.T) {
	logContent := "{\"ts\":\"2026-08-01T18:36:05.050501509+00:00\",\"provider\":\"opencode-go\",\"model\":\"deepseek-v4-flash\",\"ok\":true,\"status\":200,\"error\":null,\"upstreamUrl\":\"https://opencode.ai/zen/go/v1/chat/completions\",\"promptTokens\":100,\"completionTokens\":20,\"cachedTokens\":10,\"reasoningTokens\":5,\"costTotal\":0.001,\"conversationId\":null,\"conversationName\":null}\n" +
		"{\"ts\":\"2026-08-02T10:00:00Z\",\"provider\":\"opencode-go\",\"model\":\"deepseek-v4-flash\",\"ok\":true,\"status\":200,\"error\":null,\"upstreamUrl\":\"https://opencode.ai/zen/go/v1/chat/completions\",\"promptTokens\":200,\"completionTokens\":30,\"cachedTokens\":0,\"reasoningTokens\":0}\n" +
		"not-json{{{\n"
	writeLegacyTestEnv(t, logContent)
	r := NewMgmtRouter()
	m := getStatsMap(t, r)
	if m["totalRequests"] != float64(2) {
		t.Fatalf("totalRequests = %v, want 2", m["totalRequests"])
	}
	if m["okRequests"] != float64(2) {
		t.Fatalf("okRequests = %v, want 2", m["okRequests"])
	}
	toks, _ := m["totalTokens"].(map[string]interface{})
	if toks["input"] != float64(300) {
		t.Fatalf("totalTokens.input = %v, want 300", toks["input"])
	}
	if toks["output"] != float64(50) {
		t.Fatalf("totalTokens.output = %v, want 50", toks["output"])
	}
	if m["costUnknown"] != float64(1) {
		t.Fatalf("costUnknown = %v, want 1", m["costUnknown"])
	}
	if m["totalCost"] != float64(0.001) {
		t.Fatalf("totalCost = %v, want 0.001", m["totalCost"])
	}
}

// T2: 重复查询不重复计数（幂等导入）
func TestLegacyLog_RequeryDoesNotDuplicate(t *testing.T) {
	logContent := "{\"ts\":\"2026-08-03T12:00:00Z\",\"provider\":\"p\",\"model\":\"m\",\"ok\":true,\"status\":200,\"promptTokens\":10,\"completionTokens\":5,\"cachedTokens\":0,\"reasoningTokens\":0,\"costTotal\":0.002}\n"
	writeLegacyTestEnv(t, logContent)
	r := NewMgmtRouter()
	first := getStatsMap(t, r)
	second := getStatsMap(t, r)
	if first["totalRequests"] != float64(1) {
		t.Fatalf("first totalRequests = %v, want 1", first["totalRequests"])
	}
	if second["totalRequests"] != first["totalRequests"] {
		t.Fatalf("second totalRequests = %v, want %v", second["totalRequests"], first["totalRequests"])
	}
}

// T3: 新请求双写请求日志（旧字段形状）与 SQLite
func TestLegacyLog_NewRequestDualWritesLog(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	mock := newMockUpstream(t, nil, nil)
	defer mock.Close()
	cfgPath := writeTempConfig(t, dir, mock.URL, true)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)

	body := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	NewProxyRouter().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("POST code = %d, want 200, body=%s", w.Code, w.Body.String())
	}

	m := getStatsMap(t, NewMgmtRouter())
	if m["totalRequests"] != float64(1) {
		t.Fatalf("totalRequests = %v, want 1", m["totalRequests"])
	}

	raw, err := os.ReadFile(filepath.Join(dir, "requests.log"))
	if err != nil {
		t.Fatalf("requests.log not written: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("log lines = %d, want 1", len(lines))
	}
	var line map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &line); err != nil {
		t.Fatalf("log line not JSON: %v", err)
	}
	if line["ok"] != true {
		t.Fatalf("log ok = %v, want true", line["ok"])
	}
	if line["model"] != "gpt-4o-mini" || line["provider"] != "test-provider" {
		t.Fatalf("log provider/model = %v/%v", line["provider"], line["model"])
	}
	if line["promptTokens"] != float64(120) || line["completionTokens"] != float64(80) || line["cachedTokens"] != float64(10) {
		t.Fatalf("log tokens = %v", line)
	}
	if _, ok := line["costTotal"].(float64); !ok {
		t.Fatalf("log missing costTotal: %v", line)
	}
	if line["status"] != float64(200) {
		t.Fatalf("log status = %v, want 200", line["status"])
	}
}

// T4: 旧行在对话统计、对话明细、导出中同样可见
func TestLegacyLog_VisibleInConversationsAndExport(t *testing.T) {
	logContent := "{\"ts\":\"2026-08-04T09:00:00Z\",\"provider\":\"p\",\"model\":\"m\",\"ok\":true,\"status\":200,\"error\":null,\"upstreamUrl\":\"http://x\",\"promptTokens\":50,\"completionTokens\":10,\"cachedTokens\":5,\"reasoningTokens\":1,\"costTotal\":0.003,\"conversationId\":\"conv-old\",\"conversationName\":\"旧对话\"}\n"
	writeLegacyTestEnv(t, logContent)
	r := NewMgmtRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/stats/conversations", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("conversations code = %d", w.Code)
	}
	var convs map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &convs); err != nil {
		t.Fatalf("unmarshal conversations: %v", err)
	}
	if convs["total"] != float64(1) {
		t.Fatalf("conversations total = %v, want 1", convs["total"])
	}
	items, _ := convs["conversations"].([]interface{})
	first, _ := items[0].(map[string]interface{})
	if first["conversationId"] != "conv-old" || first["name"] != "旧对话" {
		t.Fatalf("conversation = %v", first)
	}

	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/stats/conversations/conv-old/requests", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("conversation requests code = %d", w2.Code)
	}
	var det map[string]interface{}
	if err := json.Unmarshal(w2.Body.Bytes(), &det); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if det["total"] != float64(1) {
		t.Fatalf("conversation requests total = %v, want 1", det["total"])
	}

	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/api/logs/export?format=json", nil)
	r.ServeHTTP(w3, req3)
	if w3.Code != 200 {
		t.Fatalf("export code = %d", w3.Code)
	}
	if !strings.Contains(w3.Body.String(), "2026-08-04") {
		t.Fatalf("export missing legacy row: %s", w3.Body.String()[:200])
	}
}

// T5: 日志追加后增量导入，不重复计数
func TestLegacyLog_AppendedLinesImportedOnce(t *testing.T) {
	dir := writeLegacyTestEnv(t, "{\"ts\":\"2026-08-05T09:00:00Z\",\"provider\":\"p\",\"model\":\"m\",\"ok\":true,\"status\":200,\"promptTokens\":7,\"completionTokens\":3,\"cachedTokens\":0,\"reasoningTokens\":0}\n")
	r := NewMgmtRouter()
	if m := getStatsMap(t, r); m["totalRequests"] != float64(1) {
		t.Fatalf("totalRequests = %v, want 1", m["totalRequests"])
	}
	f, err := os.OpenFile(filepath.Join(dir, "requests.log"), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	if _, err := f.WriteString("{\"ts\":\"2026-08-05T10:00:00Z\",\"provider\":\"p\",\"model\":\"m\",\"ok\":true,\"status\":200,\"promptTokens\":8,\"completionTokens\":4,\"cachedTokens\":0,\"reasoningTokens\":0}\n"); err != nil {
		t.Fatalf("append log: %v", err)
	}
	_ = f.Close()
	if m := getStatsMap(t, r); m["totalRequests"] != float64(2) {
		t.Fatalf("after append totalRequests = %v, want 2", m["totalRequests"])
	}
	if m := getStatsMap(t, r); m["totalRequests"] != float64(2) {
		t.Fatalf("requery totalRequests = %v, want 2", m["totalRequests"])
	}
}
