package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/heihei0299/pi-switch/internal/config"
)

// Contract: docs/system-contract.md §2.4 requires one outbound path for URL, headers, UA, affinity, and timeout policy.
func TestBuildOutboundRequestMergesPolicyAndMetadata(t *testing.T) {
	profileUserAgent := "profile-agent"
	globalUserAgent := "global-agent"
	outbound, err := BuildOutboundRequest(OutboundRequestPlan{
		Upstream: config.Upstream{
			BaseURL: "https://opencode.ai/zen/go/v1",
			APIKey:  "secret-key",
			Headers: map[string]string{
				"X-Channel":  "channel",
				"X-Shared":   "channel-value",
				"User-Agent": "header-agent",
			},
		},
		Path: "/v1/responses",
		ProfileHeaders: map[string]string{
			"X-Profile": "profile",
			"X-Shared":  "profile-value",
		},
		IncomingHeaders: http.Header{
			"X-Session-Affinity": []string{"session-1"},
			"X-Opencode-Client":  []string{"pi"},
			"X-Leak":             []string{"must-not-copy"},
		},
		Body:             []byte(`{"model":"m"}`),
		ContentType:      "application/json",
		Accept:           "text/event-stream",
		ProfileUserAgent: &profileUserAgent,
		GlobalUserAgent:  &globalUserAgent,
		Timeout:          7 * time.Second,
	})
	if err != nil {
		t.Fatalf("BuildOutboundRequest() error = %v", err)
	}
	if got, want := outbound.Request.URL.String(), "https://opencode.ai/zen/go/v1/responses"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
	if got := outbound.Request.Header.Get("Authorization"); got != "Bearer secret-key" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := outbound.Request.Header.Get("X-Shared"); got != "channel-value" {
		t.Fatalf("X-Shared = %q, want channel override", got)
	}
	if got := outbound.Request.Header.Get("User-Agent"); got != "profile-agent" {
		t.Fatalf("User-Agent = %q, want profile policy", got)
	}
	if got := outbound.Request.Header.Get("x-opencode-session"); got != "session-1" {
		t.Fatalf("x-opencode-session = %q", got)
	}
	if got := outbound.Request.Header.Get("x-opencode-client"); got != "pi" {
		t.Fatalf("x-opencode-client = %q", got)
	}
	if got := outbound.Request.Header.Get("X-Leak"); got != "" {
		t.Fatalf("incoming arbitrary header copied: %q", got)
	}
	if outbound.Client.Timeout != 7*time.Second {
		t.Fatalf("client timeout = %s", outbound.Client.Timeout)
	}
	if outbound.Metadata.URL != outbound.Request.URL.String() || !outbound.Metadata.HasAuthorization {
		t.Fatalf("unsafe/incomplete metadata: %+v", outbound.Metadata)
	}

	body, err := io.ReadAll(outbound.Request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); !strings.Contains(got, `"model":"m"`) {
		t.Fatalf("body = %s", got)
	}
}

func TestBuildOutboundRequestUserAgentFallback(t *testing.T) {
	global := "global-agent"
	outbound, err := BuildOutboundRequest(OutboundRequestPlan{
		Upstream:        config.Upstream{BaseURL: "https://api.example.com/v1"},
		Path:            "/v1/chat/completions",
		GlobalUserAgent: &global,
	})
	if err != nil {
		t.Fatalf("BuildOutboundRequest() error = %v", err)
	}
	if got := outbound.Request.Header.Get("User-Agent"); got != global {
		t.Fatalf("User-Agent = %q, want %q", got, global)
	}
}

func TestBuildOutboundRequestDoesNotInjectAffinityForOtherProviders(t *testing.T) {
	outbound, err := BuildOutboundRequest(OutboundRequestPlan{
		Upstream: config.Upstream{BaseURL: "https://api.example.com/v1"},
		Path:     "/v1/chat/completions",
		IncomingHeaders: http.Header{
			"X-Opencode-Session": []string{"session-1"},
			"X-Opencode-Client":  []string{"pi"},
		},
	})
	if err != nil {
		t.Fatalf("BuildOutboundRequest() error = %v", err)
	}
	if got := outbound.Request.Header.Get("X-Opencode-Session"); got != "" {
		t.Fatalf("ordinary provider received x-opencode-session = %q", got)
	}
	if got := outbound.Request.Header.Get("X-Opencode-Client"); got != "" {
		t.Fatalf("ordinary provider received x-opencode-client = %q", got)
	}
}

func TestBuildOutboundRequestUsesSelectedUpstream(t *testing.T) {
	selected := config.Upstream{
		BaseURL: "https://channel-b.example.com/v1",
		APIKey:  "channel-b-key",
		Headers: map[string]string{"X-Channel": "b"},
	}
	outbound, err := BuildOutboundRequest(OutboundRequestPlan{
		Upstream: selected,
		Path:     "/v1/chat/completions",
		Body:     []byte(`{"model":"m"}`),
	})
	if err != nil {
		t.Fatalf("BuildOutboundRequest() error = %v", err)
	}
	if got, want := outbound.Request.URL.String(), "https://channel-b.example.com/v1/chat/completions"; got != want {
		t.Fatalf("URL = %q, want %q", got, want)
	}
	if got, want := outbound.Request.Header.Get("Authorization"), "Bearer channel-b-key"; got != want {
		t.Fatalf("Authorization = %q, want %q", got, want)
	}
	if got, want := outbound.Request.Header.Get("X-Channel"), "b"; got != want {
		t.Fatalf("X-Channel = %q, want %q", got, want)
	}
}

func TestOutboundHeadersMatchAcrossInitialRetryAndStream(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	var captured []map[string]string
	var nonStreamCalls int
	var firstChannelHits int
	firstChannel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstChannelHits++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer firstChannel.Close()
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := map[string]string{}
		for _, key := range []string{"Authorization", "User-Agent", "X-Profile", "X-Channel", "X-Shared", "X-Opencode-Session", "X-Opencode-Client", "Content-Type", "Accept"} {
			headers[key] = r.Header.Get(key)
		}
		captured = append(captured, headers)
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\"}\n\ndata: [DONE]\n\n")
			return
		}
		nonStreamCalls++
		if nonStreamCalls == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"type": "invalid_request_error"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "resp-1",
			"object": "response",
			"usage":  map[string]int{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	defer mock.Close()
	opencodeBase := strings.Replace(mock.URL, "http://", "http://opencode.ai@", 1) + "/v1"
	cfg := fmt.Sprintf(`{
"version":2,
"current":"oc",
"profiles":{"oc":{"api":"openai-responses","responsesMode":"auto","baseUrl":%q,"apiKey":"profile-key","userAgent":"profile-agent","headers":{"X-Profile":"profile","X-Shared":"profile"},"upstreams":[{"name":"main","api":"openai-responses","baseUrl":%q,"apiKey":"first-channel-key","headers":{"X-Channel":"first-channel","X-Shared":"first-channel"},"models":[],"exposedModels":[]},{"name":"backup","api":"openai-responses","baseUrl":%q,"apiKey":"channel-key","headers":{"X-Channel":"channel","X-Shared":"channel"},"models":[{"id":"m","contextWindow":128000,"maxTokens":16384}],"exposedModels":["m"]}]}},
"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","conversationSource":"off","proxy":{"host":"127.0.0.1","port":43112,"userAgent":"global-agent","circuitBreaker":{"enabled":true,"failureThreshold":3,"cooldownSeconds":60}},"web":{"host":"127.0.0.1","port":43110}}
}`, firstChannel.URL, firstChannel.URL, opencodeBase)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CONFIG", filepath.Join(dir, "config.json"))
	t.Setenv("PI_SWITCH_DB", dbPath)
	router := NewProxyRouter()
	serve := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Opencode-Session", "session-1")
		req.Header.Set("X-Opencode-Client", "pi")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if w := serve(`{"model":"m","input":"hi","max_output_tokens":100}`); w.Code != http.StatusOK {
		t.Fatalf("initial/retry status = %d, body = %s", w.Code, w.Body.String())
	}
	if w := serve(`{"model":"m","input":"hi","stream":true}`); w.Code != http.StatusOK {
		t.Fatalf("stream status = %d, body = %s", w.Code, w.Body.String())
	}
	if len(captured) != 3 {
		t.Fatalf("captured %d upstream requests, want initial, retry, stream", len(captured))
	}
	if firstChannelHits != 0 {
		t.Fatalf("first channel received %d requests; selected channel credentials drifted", firstChannelHits)
	}
	for _, key := range []string{"Authorization", "User-Agent", "X-Profile", "X-Channel", "X-Shared", "X-Opencode-Session", "X-Opencode-Client", "Content-Type"} {
		if captured[0][key] != captured[1][key] || captured[1][key] != captured[2][key] {
			t.Fatalf("%s differs across requests: %#v", key, captured)
		}
	}
	if captured[0]["Authorization"] != "Bearer channel-key" {
		t.Fatalf("Authorization = %q", captured[0]["Authorization"])
	}
	if captured[0]["User-Agent"] != "profile-agent" {
		t.Fatalf("User-Agent = %q", captured[0]["User-Agent"])
	}
	if captured[0]["X-Shared"] != "channel" {
		t.Fatalf("X-Shared = %q, want channel", captured[0]["X-Shared"])
	}
	if captured[2]["Accept"] != "text/event-stream" {
		t.Fatalf("stream Accept = %q", captured[2]["Accept"])
	}
}
