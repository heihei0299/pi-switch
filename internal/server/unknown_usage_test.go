package server

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyUsagePreservesUnknownAndExplicitZero(t *testing.T) {
	for _, tc := range []struct {
		name, usage string
		known       bool
	}{
		{"unknown", `"usage":{"prompt_tokens":100,"completion_tokens":5}`, false},
		{"zero", `"usage":{"prompt_tokens":100,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":0},"completion_tokens_details":{"reasoning_tokens":0}}`, true},
		{"no usage", `"other":true`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"choices":[{"message":{"content":"hello"}}],%s}`, tc.usage)
			}))
			defer upstream.Close()
			auditFixture(t, "openai-completions", upstream.URL)
			w := httptest.NewRecorder()
			NewProxyRouter().ServeHTTP(w, auditRequest("/v1/chat/completions", false))
			if w.Code != 200 {
				t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
			}
			stats := httptest.NewRecorder()
			NewMgmtRouter().ServeHTTP(stats, httptest.NewRequest("GET", "/api/stats", nil))
			var got struct {
				RecentRequests []map[string]interface{} `json:"recentRequests"`
				CacheHitRate   string                   `json:"cacheHitRate"`
			}
			if err := json.Unmarshal(stats.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got.RecentRequests) != 1 {
				t.Fatalf("recent=%v", got.RecentRequests)
			}
			row := got.RecentRequests[0]
			for _, key := range []string{"cachedTokens", "reasoningTokens", "cached_tokens", "reasoning_tokens"} {
				if tc.known {
					if row[key] != float64(0) {
						t.Errorf("%s=%v, want explicit 0", key, row[key])
					}
				} else if row[key] != nil {
					t.Errorf("%s=%v, want unknown", key, row[key])
				}
			}
			if !tc.known && (row["cacheRate"] != "-" || got.CacheHitRate != "-") {
				t.Errorf("unknown cache rate=%v/%s", row["cacheRate"], got.CacheHitRate)
			}
			if !tc.known && row["cost"] != nil {
				t.Errorf("unknown cache distribution fabricated cost: %v", row["cost"])
			}
			exported := httptest.NewRecorder()
			NewMgmtRouter().ServeHTTP(exported, httptest.NewRequest("GET", "/api/logs/export?format=json", nil))
			var logs []map[string]interface{}
			if err := json.Unmarshal(exported.Body.Bytes(), &logs); err != nil {
				t.Fatal(err)
			}
			if len(logs) != 1 || (logs[0]["cachedTokens"] != nil && !tc.known) {
				t.Fatalf("export fabricated cached usage: %v", logs)
			}
			exported = httptest.NewRecorder()
			NewMgmtRouter().ServeHTTP(exported, httptest.NewRequest("GET", "/api/logs/export?format=csv", nil))
			records, err := csv.NewReader(strings.NewReader(exported.Body.String())).ReadAll()
			if err != nil {
				t.Fatal(err)
			}
			for i, key := range records[0] {
				if key == "cachedTokens" || key == "reasoningTokens" {
					want := ""
					if tc.known {
						want = "0"
					}
					if records[1][i] != want {
						t.Errorf("CSV %s=%q, want %q", key, records[1][i], want)
					}
				}
			}
			if tc.name == "no usage" && (row["promptTokens"] != nil || row["completionTokens"] != nil || row["cost"] != nil) {
				t.Errorf("missing usage fabricated: %v", row)
			}
		})
	}
}
