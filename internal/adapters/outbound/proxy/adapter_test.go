package proxy

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/heihei0299/pi-switch/internal/domain/provider"
)

// Contract: docs/system-contract.md §2.4 requires one outbound path for URL, headers, UA, affinity, and timeout policy.
func TestBuildOutboundRequestMergesPolicyAndMetadata(t *testing.T) {
	profileUserAgent := "profile-agent"
	globalUserAgent := "global-agent"
	outbound, err := BuildOutboundRequest(OutboundRequestPlan{
		Upstream: provider.Upstream{
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
		Upstream:        provider.Upstream{BaseURL: "https://api.example.com/v1"},
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
		Upstream: provider.Upstream{BaseURL: "https://api.example.com/v1"},
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
	selected := provider.Upstream{
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
