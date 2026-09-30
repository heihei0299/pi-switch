package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestUnsupportedStreamRejectedBeforeUpstream(t *testing.T) {
	for _, tc := range []struct{ api, path string }{
		{"anthropic-messages", "/v1/chat/completions"},
		{"openai-completions", "/v1/messages"},
	} {
		t.Run(tc.api, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer upstream.Close()
			auditFixture(t, tc.api, upstream.URL)
			w := httptest.NewRecorder()
			NewProxyRouter().ServeHTTP(w, auditRequest(tc.path, true))
			if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "not_supported") {
				t.Errorf("HTTP %d, want explicit unsupported error: %s", w.Code, w.Body.String())
			}
			if calls.Load() != 0 {
				t.Errorf("unsupported stream made %d upstream calls", calls.Load())
			}
		})
	}
}
