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

func TestMessagesUsageDoesNotGuessUnknownCache(t *testing.T) {
	for _, tc := range []struct {
		name, details    string
		uncached, cached interface{}
	}{
		{"unknown", "", nil, nil},
		{"zero", `,"prompt_tokens_details":{"cached_tokens":0}`, float64(100), float64(0)},
		{"known", `,"prompt_tokens_details":{"cached_tokens":25}`, float64(75), float64(25)},
	} {
		for _, retry := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/retry=%t", tc.name, retry), func(t *testing.T) {
				calls := 0
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					w.Header().Set("Content-Type", "application/json")
					if retry && calls == 1 {
						w.WriteHeader(http.StatusBadRequest)
						fmt.Fprint(w, `{"error":{"type":"invalid_request_error"}}`)
						return
					}
					fmt.Fprintf(w, `{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":5%s}}`, tc.details)
				}))
				defer upstream.Close()
				auditFixture(t, "openai-completions", upstream.URL)
				w := httptest.NewRecorder()
				NewProxyRouter().ServeHTTP(w, auditRequest("/v1/messages", false))
				if w.Code != http.StatusOK {
					t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
				}
				var response map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				mapped, _ := response["usage"].(map[string]interface{})
				if mapped["input_tokens"] != tc.uncached || mapped["output_tokens"] != float64(5) || mapped["cache_read_input_tokens"] != tc.cached {
					t.Errorf("Messages usage=%v, want ordinary input %v/output 5/cache read %v", mapped, tc.uncached, tc.cached)
				}
				if tc.cached == nil && mapped["cache_creation_input_tokens"] != nil {
					t.Errorf("unknown cache distribution fabricated cache write: %v", mapped)
				} else if tc.cached != nil && mapped["cache_creation_input_tokens"] != float64(0) {
					t.Errorf("known Chat usage should have zero cache write: %v", mapped)
				}
				stats := getStatsMap(t, NewMgmtRouter())
				rows := stats["recentRequests"].([]interface{})
				if len(rows) != 1 {
					t.Fatalf("request facts=%v, want one request", rows)
				}
				row := rows[0].(map[string]interface{})
				if row["promptTokens"] != float64(100) || row["completionTokens"] != float64(5) || row["cachedTokens"] != tc.cached {
					t.Errorf("Stats lost raw upstream usage facts: %v", row)
				}
				if tc.cached == nil && row["cost"] != nil {
					t.Errorf("unknown cache distribution fabricated consumption: %v", row)
				}
			})
		}
	}
}
