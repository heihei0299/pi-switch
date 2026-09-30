package server

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func TestAnthropicCacheUsageAndCostThroughProxy(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":100,\"cache_creation_input_tokens\":200,\"cache_read_input_tokens\":700,\"output_tokens\":0}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":50}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"type":"message","role":"assistant","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":100,"cache_creation_input_tokens":200,"cache_read_input_tokens":700,"output_tokens":50}}`)
			}))
			defer upstream.Close()
			auditFixture(t, "anthropic-messages", upstream.URL)
			if err := config.UpdateAtPath(config.ResolvePath(), func(cfg *config.PiSwitchConfig) error {
				p := cfg.Profiles["audit"]
				p.Upstreams[0].Models[0].Cost.CacheWrite = 3.75
				cfg.Profiles["audit"] = p
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			NewProxyRouter().ServeHTTP(w, auditRequest("/v1/messages", stream))
			if w.Code != 200 {
				t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
			}
			stats := httptest.NewRecorder()
			NewMgmtRouter().ServeHTTP(stats, httptest.NewRequest("GET", "/api/stats", nil))
			var got struct {
				Recent []struct {
					Prompt int64    `json:"promptTokens"`
					Cached int64    `json:"cachedTokens"`
					Cost   *float64 `json:"cost"`
				} `json:"recentRequests"`
				CacheRate string `json:"cacheHitRate"`
			}
			if err := json.Unmarshal(stats.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got.Recent) != 1 {
				t.Fatalf("stats=%s", stats.Body.String())
			}
			row := got.Recent[0]
			if row.Prompt != 1000 || row.Cached != 700 || got.CacheRate != "70.0%" || row.Cost == nil || math.Abs(*row.Cost-0.00201) > 1e-10 {
				t.Fatalf("incorrect cache usage/cost: %+v, cacheRate=%s", row, got.CacheRate)
			}
		})
	}
}
