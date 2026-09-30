package server

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/store"
)

func TestActualRequestStatusAndErrorReachStatsDetailAndExports(t *testing.T) {
	for _, status := range []int{401, 429, 503, 201} {
		for _, stream := range []bool{false, true} {
			if stream && status == 201 {
				continue
			}
			t.Run(fmt.Sprintf("status=%d/stream=%t", status, stream), func(t *testing.T) {
				body := fmt.Sprintf(`{"error":{"message":"quota, exhausted","code":%d}}`, status)
				var wantError interface{} = body
				if status == 201 {
					body = `{"choices":[{"message":{"content":"hello"}}]}`
					wantError = nil
				}
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					fmt.Fprint(w, body)
				}))
				defer upstream.Close()
				auditFixture(t, "openai-completions", upstream.URL)
				if err := config.UpdateAtPath(config.ResolvePath(), func(cfg *config.PiSwitchConfig) error {
					cfg.Settings.ConversationSource = "proxy"
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				w := httptest.NewRecorder()
				req := auditRequest("/v1/chat/completions", stream)
				req.Header.Set("X-Conversation-Id", "facts")
				NewProxyRouter().ServeHTTP(w, req)
				if w.Code != status {
					t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
				}
				check := func(label string, row map[string]interface{}) {
					t.Helper()
					if row["status"] != float64(status) || row["error"] != wantError || row["upstream_url"] != upstream.URL+"/v1/chat/completions" {
						t.Errorf("%s lost actual facts: %v", label, row)
					}
				}
				mgmt := NewMgmtRouter()
				stats := getStatsMap(t, mgmt)
				check("recent", stats["recentRequests"].([]interface{})[0].(map[string]interface{}))
				detail := httptest.NewRecorder()
				mgmt.ServeHTTP(detail, httptest.NewRequest("GET", "/api/stats/conversations/facts/requests", nil))
				var got struct {
					Requests []map[string]interface{} `json:"requests"`
				}
				if err := json.Unmarshal(detail.Body.Bytes(), &got); err != nil || len(got.Requests) != 1 {
					t.Fatalf("detail=%s, error=%v", detail.Body.String(), err)
				}
				check("detail", got.Requests[0])
				exported := httptest.NewRecorder()
				mgmt.ServeHTTP(exported, httptest.NewRequest("GET", "/api/logs/export?format=json", nil))
				var logs []map[string]interface{}
				if err := json.Unmarshal(exported.Body.Bytes(), &logs); err != nil || len(logs) != 1 {
					t.Fatalf("export=%s, error=%v", exported.Body.String(), err)
				}
				check("JSON", logs[0])
				exported = httptest.NewRecorder()
				mgmt.ServeHTTP(exported, httptest.NewRequest("GET", "/api/logs/export?format=csv", nil))
				records, err := csv.NewReader(strings.NewReader(exported.Body.String())).ReadAll()
				if err != nil || len(records) != 2 {
					t.Fatalf("CSV=%s, error=%v", exported.Body.String(), err)
				}
				wantCSV := map[string]string{"status": strconv.Itoa(status), "error": "", "upstream_url": upstream.URL + "/v1/chat/completions"}
				if wantError != nil {
					wantCSV["error"] = body
				}
				for i, key := range records[0] {
					if want, ok := wantCSV[key]; ok && records[1][i] != want {
						t.Errorf("CSV %s=%q, want %q", key, records[1][i], want)
					}
				}
			})
		}
	}
}

func TestLegacyImportPreservesErrorFactsAndUnknownStatus(t *testing.T) {
	writeLegacyTestEnv(t, `{"ts":"2026-09-10T12:00:00Z","provider":"p","model":"m","ok":false,"status":503,"error":"overloaded","upstreamUrl":"https://example.test/chat","conversationId":"legacy"}`+"\n"+`{"ts":"2026-09-10T12:01:00Z","provider":"p","model":"m","ok":false,"conversationId":"legacy"}`+"\n")
	t.Cleanup(store.Close)
	importLegacyForTest(t)
	stats := getStatsMap(t, NewMgmtRouter())
	rows := stats["recentRequests"].([]interface{})
	unknown := rows[0].(map[string]interface{})
	known := rows[1].(map[string]interface{})
	if unknown["status"] != nil || unknown["error"] != nil || unknown["upstream_url"] != nil {
		t.Errorf("legacy unknown facts guessed: %v", unknown)
	}
	if known["status"] != float64(503) || known["error"] != "overloaded" || known["upstream_url"] != "https://example.test/chat" {
		t.Errorf("legacy actual facts lost: %v", known)
	}
}
