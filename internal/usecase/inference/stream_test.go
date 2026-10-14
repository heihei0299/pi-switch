package inference

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/domain/protocol"
	"github.com/heihei0299/pi-switch/internal/domain/provider"
)

type relayTestBody struct {
	chunks [][]byte
	err    error
	reads  int
	closed bool
	cancel context.CancelFunc
}

func (b *relayTestBody) Read(p []byte) (int, error) {
	b.reads++
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	if len(b.chunks) > 0 {
		chunk := b.chunks[0]
		b.chunks = b.chunks[1:]
		return copy(p, chunk), b.err
	}
	if b.err != nil {
		err := b.err
		b.err = nil
		return 0, err
	}
	return 0, io.EOF
}

func (b *relayTestBody) Close() error { b.closed = true; return nil }

type relayTestRecorder struct {
	*httptest.ResponseRecorder
	flushes int
}

func newRelayTestRecorder() *relayTestRecorder {
	return &relayTestRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (w *relayTestRecorder) SetHeader(key, value string) { w.ResponseRecorder.Header().Set(key, value) }

func (w *relayTestRecorder) Flush() {
	w.flushes++
	w.ResponseRecorder.Flush()
}

func TestRelayStreamPassThroughPreservesBytesHeadersAndUsage(t *testing.T) {
	chunks := [][]byte{
		[]byte("data: {\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"cached_tokens\":0}}\n\n"),
		[]byte("data: [DONE]\n\n"),
	}
	body := &relayTestBody{chunks: chunks}
	writer := newRelayTestRecorder()
	input := StreamInput{
		UpstreamStatus: http.StatusOK,
		UpstreamHeaders: Header{
			"Content-Length": {"99"},
			"X-Upstream":     {"kept"},
		},
		Body:           body,
		UpstreamFormat: protocol.FormatOpenAIChat,
		ModelEntry: &provider.ModelEntry{Cost: &provider.ModelCost{
			Input: 1, Output: 2, CacheRead: 1,
		}},
	}

	facts := RelayStream(context.Background(), input, writer)
	if !facts.Success || facts.Status != http.StatusOK {
		t.Fatalf("stream facts = %+v, want success/200", facts)
	}
	if got, want := writer.Body.String(), string(chunks[0])+string(chunks[1]); got != want {
		t.Fatalf("relayed bytes = %q, want %q", got, want)
	}
	if writer.Code != http.StatusOK || writer.Header().Get("X-Upstream") != "kept" || writer.Header().Get("Content-Length") != "" {
		t.Fatalf("status/headers = %d/%v", writer.Code, writer.Header())
	}
	if writer.Header().Get("Content-Type") != "text/event-stream" || writer.Header().Get("Cache-Control") != "no-cache" || writer.flushes != 2 {
		t.Fatalf("stream headers/flush count = %v/%d", writer.Header(), writer.flushes)
	}
	if facts.Usage == nil || facts.Usage.PromptTokens != 10 || facts.Usage.CompletionTokens != 5 || !facts.Usage.CachedTokensKnown {
		t.Fatalf("stream usage = %+v", facts.Usage)
	}
	if facts.Cost == nil || *facts.Cost < 0.000019999 || *facts.Cost > 0.000020001 {
		t.Fatalf("stream cost = %v, want 0.00002", facts.Cost)
	}
	if !body.closed {
		t.Fatal("upstream body was not closed")
	}
}

func TestRelayStreamWritesTheUpstreamStatus(t *testing.T) {
	writer := newRelayTestRecorder()
	RelayStream(context.Background(), StreamInput{
		UpstreamStatus: http.StatusPartialContent,
		Body:           &relayTestBody{chunks: [][]byte{[]byte("data: [DONE]\n\n")}},
		UpstreamFormat: protocol.FormatOpenAIChat,
	}, writer)
	if writer.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", writer.Code)
	}
}

func TestRelayStreamConversionRequiresSuccessfulTerminalEvent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		readErr error
		success bool
		done    bool
	}{
		{"completed", "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{}}\n\n", io.EOF, true, true},
		{"missing terminal", "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n", io.EOF, false, false},
		{"response failed", "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"message\":\"boom\"}}}\n\n", io.EOF, false, false},
		{"read error", "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n", io.ErrUnexpectedEOF, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer := newRelayTestRecorder()
			facts := RelayStream(context.Background(), StreamInput{
				UpstreamStatus:   http.StatusOK,
				Body:             &relayTestBody{chunks: [][]byte{[]byte(tc.payload)}, err: tc.readErr},
				UpstreamFormat:   protocol.FormatOpenAIResponses,
				DownstreamFormat: protocol.FormatOpenAIChat,
				Converter:        protocol.NewResponsesSseToChat("model"),
			}, writer)
			if facts.Success != tc.success {
				t.Fatalf("success = %t, facts=%+v", facts.Success, facts)
			}
			if got := strings.Contains(writer.Body.String(), "data: [DONE]"); got != tc.done {
				t.Fatalf("DONE presence = %t, body=%s", got, writer.Body.String())
			}
			if tc.success && facts.Status != http.StatusOK || !tc.success && facts.Status != http.StatusBadGateway {
				t.Fatalf("logged status = %d, success=%t", facts.Status, facts.Success)
			}
		})
	}
}

func TestRelayStreamDoesNotFabricateUnknownUsage(t *testing.T) {
	facts := RelayStream(context.Background(), StreamInput{
		UpstreamStatus: http.StatusOK,
		Body:           &relayTestBody{chunks: [][]byte{[]byte("data: [DONE]\n\n")}},
		UpstreamFormat: protocol.FormatOpenAIChat,
		ModelEntry:     &provider.ModelEntry{Cost: &provider.ModelCost{Input: 1, Output: 1, CacheRead: 1}},
	}, newRelayTestRecorder())
	if !facts.Success || facts.Usage != nil || facts.Cost != nil {
		t.Fatalf("unknown usage facts = %+v, want success with unknown usage/cost", facts)
	}
}

func TestRelayStreamPassThroughFailsOnReadErrorOrFailureEvent(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		format        protocol.Format
		readErr       error
	}{
		{"read error", "data: {\"choices\":[]}\n\n", protocol.FormatOpenAIChat, io.ErrUnexpectedEOF},
		{"response failed", "event: response.failed\ndata: {\"type\":\"response.failed\"}\n\n", protocol.FormatOpenAIResponses, io.EOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := RelayStream(context.Background(), StreamInput{
				UpstreamStatus: http.StatusOK,
				Body: &relayTestBody{
					chunks: [][]byte{[]byte(tc.payload)},
					err:    tc.readErr,
				},
				UpstreamFormat: tc.format,
			}, newRelayTestRecorder())
			if facts.Success || facts.Status != http.StatusBadGateway {
				t.Fatalf("facts = %+v, want failed/502", facts)
			}
		})
	}
}

func TestRelayStreamCancellationStopsBeforeTerminalWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	body := &relayTestBody{chunks: [][]byte{[]byte("data: [DONE]\n\n")}, err: context.Canceled, cancel: cancel}
	writer := newRelayTestRecorder()
	facts := RelayStream(ctx, StreamInput{
		UpstreamStatus: http.StatusOK,
		Body:           body,
		UpstreamFormat: protocol.FormatOpenAIChat,
	}, writer)
	if facts.Success || facts.Status != 499 || writer.Body.Len() != 0 || !body.closed {
		t.Fatalf("canceled stream facts=%+v body=%q closed=%t", facts, writer.Body.String(), body.closed)
	}
}

type relayFailingWriter struct {
	header http.Header
	writes int
}

func (w *relayFailingWriter) SetHeader(key, value string) {
	if w.header == nil {
		w.header = make(http.Header)
	}
	w.header.Set(key, value)
}
func (*relayFailingWriter) WriteHeader(int) {}
func (*relayFailingWriter) Flush()          {}
func (w *relayFailingWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, errors.New("client connection closed")
}

func TestRelayStreamDownstreamWriteFailureClosesUpstream(t *testing.T) {
	body := &relayTestBody{chunks: [][]byte{[]byte("data: [DONE]\n\n"), []byte("data: later\n\n")}}
	writer := &relayFailingWriter{}
	facts := RelayStream(context.Background(), StreamInput{
		UpstreamStatus: http.StatusOK,
		Body:           body,
		UpstreamFormat: protocol.FormatOpenAIChat,
	}, writer)
	if facts.Success || facts.Status != 499 || writer.writes != 1 || body.reads != 1 || !body.closed {
		t.Fatalf("write failure facts=%+v writes=%d reads=%d closed=%t", facts, writer.writes, body.reads, body.closed)
	}
}

type relayDiscardWriter struct {
	writes  int
	bytes   int
	flushes int
}

func (*relayDiscardWriter) SetHeader(string, string) {}
func (*relayDiscardWriter) WriteHeader(int)          {}
func (w *relayDiscardWriter) Flush()                 { w.flushes++ }
func (w *relayDiscardWriter) Write(p []byte) (int, error) {
	w.writes++
	w.bytes += len(p)
	return len(p), nil
}

type generatedSSEBody struct {
	remaining int
	frame     []byte
	done      bool
	closed    bool
}

func (b *generatedSSEBody) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) && b.remaining > 0 {
		if len(b.frame) == 0 {
			b.frame = []byte("data: {}\n\n")
		}
		copyN := min(len(p)-n, min(b.remaining, len(b.frame)))
		copy(p[n:], b.frame[:copyN])
		n += copyN
		b.remaining -= copyN
		b.frame = b.frame[copyN:]
		if len(b.frame) == 0 && b.remaining > 0 {
			b.frame = []byte("data: {}\n\n")
		}
	}
	if n > 0 {
		return n, nil
	}
	if !b.done {
		b.done = true
		return copy(p, []byte("data: [DONE]\n\n")), nil
	}
	return 0, io.EOF
}

func (b *generatedSSEBody) Close() error { b.closed = true; return nil }

type generatedBytesBody struct {
	remaining int
	value     byte
	closed    bool
}

func (b *generatedBytesBody) Read(p []byte) (int, error) {
	if b.remaining == 0 {
		return 0, io.EOF
	}
	n := min(len(p), b.remaining)
	for i := 0; i < n; i++ {
		p[i] = b.value
	}
	b.remaining -= n
	return n, nil
}

func (b *generatedBytesBody) Close() error { b.closed = true; return nil }

type heapSamplingWriter struct {
	peakHeap uint64
	bytes    int
	sampled  int
}

func (w *heapSamplingWriter) SetHeader(string, string) {}
func (*heapSamplingWriter) WriteHeader(int)            {}
func (*heapSamplingWriter) Flush()                     {}
func (w *heapSamplingWriter) Write(p []byte) (int, error) {
	w.bytes += len(p)
	if w.bytes-w.sampled >= 1<<20 {
		w.sampled = w.bytes
		runtime.GC()
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		if stats.HeapAlloc > w.peakHeap {
			w.peakHeap = stats.HeapAlloc
		}
	}
	return len(p), nil
}

func TestRelayStreamRejectsOversizedSSEFrame(t *testing.T) {
	body := &generatedBytesBody{remaining: maxSSEFrameBytes + 4096, value: 'x'}
	facts := RelayStream(context.Background(), StreamInput{
		UpstreamStatus: http.StatusOK,
		Body:           body,
		UpstreamFormat: protocol.FormatOpenAIChat,
	}, &relayDiscardWriter{})
	if facts.Success || facts.Status != http.StatusBadGateway || facts.Error != errSSEFrameTooLarge.Error() || !body.closed {
		t.Fatalf("oversized stream facts=%+v closed=%t", facts, body.closed)
	}
}

func TestRelayStreamConversionRejectsOversizedSSEFrame(t *testing.T) {
	body := &generatedBytesBody{remaining: maxSSEFrameBytes + 4096, value: 'x'}
	facts := RelayStream(context.Background(), StreamInput{
		UpstreamStatus:   http.StatusOK,
		Body:             body,
		UpstreamFormat:   protocol.FormatOpenAIResponses,
		DownstreamFormat: protocol.FormatOpenAIChat,
		Converter:        protocol.NewResponsesSseToChat("model"),
	}, &relayDiscardWriter{})
	if facts.Success || facts.Status != http.StatusBadGateway || facts.Error != errSSEFrameTooLarge.Error() || !body.closed {
		t.Fatalf("oversized converted stream facts=%+v closed=%t", facts, body.closed)
	}
}

func TestRelayStreamLongPassThroughDoesNotRetainWholeResponse(t *testing.T) {
	const streamBytes = (32 << 20) / 10 * 10
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	writer := &heapSamplingWriter{}
	facts := RelayStream(context.Background(), StreamInput{
		UpstreamStatus: http.StatusOK,
		Body:           &generatedSSEBody{remaining: streamBytes},
		UpstreamFormat: protocol.FormatOpenAIChat,
	}, writer)
	if !facts.Success {
		t.Fatalf("long stream facts=%+v", facts)
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if after.HeapAlloc > writer.peakHeap {
		writer.peakHeap = after.HeapAlloc
	}
	delta := int64(writer.peakHeap) - int64(before.HeapAlloc)
	t.Logf("stream_bytes=%d peak_heap_delta=%d", streamBytes, delta)
	if delta > 8<<20 {
		t.Fatalf("peak heap grew by %d bytes for %d-byte stream", delta, streamBytes)
	}
}

func BenchmarkRelayStreamLongPassThrough(b *testing.B) {
	for _, streamBytes := range []int{(1 << 20) / 10 * 10, (32 << 20) / 10 * 10} {
		b.Run(fmt.Sprintf("%dKiB", streamBytes>>10), func(b *testing.B) {
			b.ReportMetric(float64(streamBytes), "stream_bytes")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				facts := RelayStream(context.Background(), StreamInput{
					UpstreamStatus: http.StatusOK,
					Body:           &generatedSSEBody{remaining: streamBytes},
					UpstreamFormat: protocol.FormatOpenAIChat,
				}, &relayDiscardWriter{})
				if !facts.Success {
					b.Fatalf("long stream facts=%+v", facts)
				}
			}
		})
	}
}
