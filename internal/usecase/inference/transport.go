package inference

import (
	"context"
	"io"
	"strings"
)

// Header is the transport-neutral subset of response metadata needed by inference.
type Header map[string][]string

func (h Header) Set(key, value string) {
	for name := range h {
		if strings.EqualFold(name, key) {
			h[name] = []string{value}
			return
		}
	}
	h[key] = []string{value}
}

func (h Header) Get(key string) string {
	for name, values := range h {
		if strings.EqualFold(name, key) && len(values) != 0 {
			return values[0]
		}
	}
	return ""
}

type OutboundResponse struct {
	Status  int
	Headers Header
	Body    io.ReadCloser
	URL     string
}

// OutboundRequest hides request construction and transport execution from the use case.
type OutboundRequest interface {
	Do(context.Context) (OutboundResponse, error)
}

type StreamSink interface {
	Write([]byte) (int, error)
	Flush()
}

// StreamResponseStarter applies transport metadata before the first SSE byte.
type StreamResponseStarter func(status int, headers Header)
