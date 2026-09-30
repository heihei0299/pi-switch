package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientCancellationStopsUpstream(t *testing.T) {
	for _, tc := range []struct {
		name, api, path string
		stream, retry   bool
	}{
		{"nonstream", "openai-completions", "/v1/chat/completions", false, false},
		{"nonstream converted", "openai-responses", "/v1/chat/completions", false, false},
		{"retry", "openai-completions", "/v1/chat/completions", false, true},
		{"passthrough SSE", "openai-completions", "/v1/chat/completions", true, false},
		{"Responses to Chat SSE", "openai-responses", "/v1/chat/completions", true, false},
		{"Chat to Responses SSE", "openai-completions", "/v1/responses", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			started, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			cleanup := func() { once.Do(func() { close(release) }) }
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.retry && calls.Add(1) == 1 {
					w.WriteHeader(400)
					fmt.Fprint(w, `{"error":{"type":"invalid_request_error"}}`)
					return
				}
				if tc.stream {
					w.Header().Set("Content-Type", "text/event-stream")
					if tc.api == "openai-responses" {
						fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"first\"}\n\n")
					} else {
						fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n")
					}
					w.(http.Flusher).Flush()
				}
				if !tc.stream {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"partial":`)
					w.(http.Flusher).Flush()
				}
				close(started)
				select {
				case <-r.Context().Done():
					close(canceled)
				case <-release:
				}
			}))
			defer upstream.Close()
			defer cleanup()
			auditFixture(t, tc.api, upstream.URL)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { NewProxyRouter().ServeHTTP(w, auditRequest(tc.path, tc.stream).WithContext(ctx)); close(done) }()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("upstream did not start")
			}
			cancel()
			select {
			case <-canceled:
			case <-time.After(time.Second):
				t.Error("upstream did not receive client cancellation")
			}
			cleanup()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("proxy did not stop")
			}
			if !tc.stream && w.Body.Len() != 0 {
				t.Errorf("canceled request returned partial/error body: %s", w.Body.String())
			}
			if tc.stream && (strings.Contains(w.Body.String(), "data: [DONE]") || strings.Contains(w.Body.String(), "response.completed")) {
				t.Error("canceled stream emitted successful terminal event")
			}
		})
	}
}

type failedClientWriter struct {
	*httptest.ResponseRecorder
	writes atomic.Int32
}

func (w *failedClientWriter) Write([]byte) (int, error) {
	w.writes.Add(1)
	return 0, errors.New("client connection closed")
}

func TestClientWriteFailureStopsUpstream(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/v1/responses"} {
		t.Run(path, func(t *testing.T) {
			canceled, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			cleanup := func() { once.Do(func() { close(release) }) }
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n")
				w.(http.Flusher).Flush()
				select {
				case <-r.Context().Done():
					close(canceled)
				case <-release:
				}
			}))
			defer upstream.Close()
			defer cleanup()
			auditFixture(t, "openai-completions", upstream.URL)
			w := &failedClientWriter{ResponseRecorder: httptest.NewRecorder()}
			done := make(chan struct{})
			go func() { NewProxyRouter().ServeHTTP(w, auditRequest(path, true)); close(done) }()
			select {
			case <-canceled:
			case <-time.After(time.Second):
				t.Error("failed downstream write left upstream running")
			}
			cleanup()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("proxy did not stop")
			}
			if n := w.writes.Load(); n != 1 {
				t.Errorf("downstream writes=%d, want 1", n)
			}
		})
	}
}

type cancellationTransport func(*http.Request) (*http.Response, error)

func (f cancellationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type terminalOnCancelBody struct {
	cancel context.CancelFunc
	closed bool
}

func (b *terminalOnCancelBody) Read(p []byte) (int, error) {
	b.cancel()
	return copy(p, "data: [DONE]\n\n"), context.Canceled
}
func (b *terminalOnCancelBody) Close() error { b.closed = true; return nil }

func TestCanceledStreamDoesNotForwardTerminalChunk(t *testing.T) {
	auditFixture(t, "openai-completions", "http://upstream.test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := &terminalOnCancelBody{cancel: cancel}
	original := http.DefaultTransport
	http.DefaultTransport = cancellationTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: body, Request: r}, nil
	})
	defer func() { http.DefaultTransport = original }()
	w := httptest.NewRecorder()
	NewProxyRouter().ServeHTTP(w, auditRequest("/v1/chat/completions", true).WithContext(ctx))
	if w.Body.Len() != 0 {
		t.Errorf("canceled stream forwarded terminal chunk: %s", w.Body.String())
	}
	if !body.closed {
		t.Error("upstream body not closed")
	}
}

type eofOnCancelBody struct {
	cancel  context.CancelFunc
	payload string
	closed  bool
}

func (b *eofOnCancelBody) Read(p []byte) (int, error) {
	b.cancel()
	return copy(p, b.payload), io.EOF
}

func (b *eofOnCancelBody) Close() error { b.closed = true; return nil }

func TestNonStreamingCancellationAtEOFIsRecordedAsFailure(t *testing.T) {
	for _, tc := range []struct {
		name, api, payload string
		retry              bool
	}{
		{"first", "openai-completions", `{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}]}`, false},
		{"first converted", "openai-responses", `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}]}`, false},
		{"retry", "openai-completions", `{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}]}`, true},
		{"retry converted", "openai-responses", `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auditFixture(t, tc.api, "http://upstream.test")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			body := &eofOnCancelBody{cancel: cancel, payload: tc.payload}
			original := http.DefaultTransport
			defer func() { http.DefaultTransport = original }()
			calls := 0
			http.DefaultTransport = cancellationTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				status, responseBody := http.StatusOK, io.ReadCloser(body)
				if tc.retry && calls == 1 {
					status = http.StatusBadRequest
					responseBody = io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error"}}`))
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: responseBody, Request: r}, nil
			})
			w := httptest.NewRecorder()
			NewProxyRouter().ServeHTTP(w, auditRequest("/v1/chat/completions", false).WithContext(ctx))
			if w.Body.Len() != 0 {
				t.Errorf("canceled request returned response body: %s", w.Body.String())
			}
			if !body.closed {
				t.Error("upstream body not closed")
			}
			stats := getStatsMap(t, NewMgmtRouter())
			rows := stats["recentRequests"].([]interface{})
			if len(rows) != 1 {
				t.Fatalf("request facts=%v, want one canceled request", rows)
			}
			row := rows[0].(map[string]interface{})
			if row["ok"] != false || row["status"] != float64(499) || row["error"] != context.Canceled.Error() {
				t.Errorf("canceled request facts=%v, want failure/499/context canceled", row)
			}
		})
	}
}
