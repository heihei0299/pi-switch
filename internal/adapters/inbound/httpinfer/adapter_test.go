package httpinfer

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/heihei0299/pi-switch/internal/usecase/inference"
)

type memoryResponseWriter struct {
	header  http.Header
	body    bytes.Buffer
	status  int
	flushes int
}

func (w *memoryResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *memoryResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *memoryResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(p)
}

func (w *memoryResponseWriter) Flush() { w.flushes++ }

func TestStreamResponseAdapterOwnsHTTPResponseOperations(t *testing.T) {
	w := &memoryResponseWriter{}
	NewStreamResponseStarter(w)(206, inference.Header{
		"Content-Length":    {"99"},
		"Transfer-Encoding": {"chunked"},
		"Connection":        {"close"},
		"X-Upstream":        {"kept"},
	})
	sink := NewStreamWriter(w)
	if _, err := sink.Write([]byte("data: [DONE]\n\n")); err != nil {
		t.Fatal(err)
	}
	sink.Flush()

	if w.status != 206 || w.Header().Get("X-Upstream") != "kept" || w.Header().Get("Content-Length") != "" || w.Header().Get("Transfer-Encoding") != "" {
		t.Fatalf("status/headers = %d/%v", w.status, w.Header())
	}
	if w.Header().Get("Content-Type") != "text/event-stream" || w.Header().Get("Cache-Control") != "no-cache" || w.Header().Get("Connection") != "keep-alive" || w.flushes != 1 {
		t.Fatalf("stream headers/flushes = %v/%d", w.Header(), w.flushes)
	}
	if w.body.String() != "data: [DONE]\n\n" {
		t.Fatalf("body = %q", w.body.String())
	}
}

func TestWriteResponseAdapterWritesUpstreamResponse(t *testing.T) {
	w := &memoryResponseWriter{}
	WriteResponse(w, 429, inference.Header{"Content-Type": {"application/json"}, "X-Upstream": {"kept"}}, []byte(`{"error":"rate limited"}`))
	if w.status != 429 || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("X-Upstream") != "kept" || w.body.String() != `{"error":"rate limited"}` {
		t.Fatalf("response = status %d headers %v body %q", w.status, w.Header(), w.body.String())
	}
}
