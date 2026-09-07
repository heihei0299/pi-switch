package server

import (
	"net/http"
	"testing"
)

func TestApplyOpenCodeSessionAffinityHeader(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string
		incoming   http.Header
		initial    string
		want       string
		wantAbsent bool
	}{
		{
			name:     "maps session affinity for OpenCode",
			baseURL:  "https://opencode.ai/zen/go/v1",
			incoming: http.Header{"X-Session-Affinity": []string{"session-from-pi"}},
			want:     "session-from-pi",
		},
		{
			name:     "prefers explicit OpenCode session",
			baseURL:  "https://opencode.ai/zen/go/v1",
			incoming: http.Header{"X-Opencode-Session": []string{"explicit"}, "X-Session-Affinity": []string{"fallback"}},
			initial:  "static",
			want:     "explicit",
		},
		{
			name:     "falls back to client request id",
			baseURL:  "https://opencode.ai/zen/go/v1",
			incoming: http.Header{"X-Client-Request-Id": []string{"request-id"}},
			want:     "request-id",
		},
		{
			name:       "does not inject for other upstreams",
			baseURL:    "https://api.example.com/v1",
			incoming:   http.Header{"X-Session-Affinity": []string{"third-party-session"}},
			initial:    "static",
			want:       "static",
			wantAbsent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outgoing := make(http.Header)
			if tt.initial != "" {
				outgoing.Set("x-opencode-session", tt.initial)
			}
			applyOpenCodeSessionAffinityHeader(tt.baseURL, tt.incoming, outgoing)
			got := outgoing.Get("x-opencode-session")
			if got != tt.want {
				t.Fatalf("x-opencode-session = %q, want %q", got, tt.want)
			}
			if tt.wantAbsent && got != "" {
				t.Fatalf("x-opencode-session = %q, want absent", got)
			}
		})
	}
}
