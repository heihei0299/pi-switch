package httpinfer

import (
	"net/http"
	"strings"

	"github.com/heihei0299/pi-switch/internal/usecase/inference"
)

func NewStreamWriter(w http.ResponseWriter) inference.StreamSink {
	return streamWriter{writer: w}
}

type streamWriter struct{ writer http.ResponseWriter }

func (w streamWriter) Write(data []byte) (int, error) { return w.writer.Write(data) }
func (w streamWriter) Flush() {
	if flusher, ok := w.writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

func NewStreamResponseStarter(w http.ResponseWriter) inference.StreamResponseStarter {
	return func(status int, headers inference.Header) {
		for key, values := range headers {
			for _, value := range values {
				if strings.EqualFold(key, "Content-Length") || strings.EqualFold(key, "Transfer-Encoding") || strings.EqualFold(key, "Connection") {
					continue
				}
				w.Header().Set(key, value)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(status)
	}
}

func WriteResponse(w http.ResponseWriter, status int, headers inference.Header, body []byte) {
	for key, values := range headers {
		for _, value := range values {
			w.Header().Set(key, value)
		}
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
