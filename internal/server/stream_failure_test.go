package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/store"
	"github.com/heihei0299/pi-switch/internal/translator"
)

type scriptedBody struct {
	payload []byte
	err     error
	read    bool
}

func (b *scriptedBody) Read(p []byte) (int, error) {
	if b.read {
		return 0, b.err
	}
	b.read = true
	return copy(p, b.payload), b.err
}

func (b *scriptedBody) Close() error { return nil }

func runConvertedStream(t *testing.T, payload string, readErr error) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	writeLegacyTestEnv(t, "")
	t.Cleanup(store.Close)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "http://proxy.test/v1/chat/completions", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       &scriptedBody{payload: []byte(payload), err: readErr},
		Request:    c.Request,
	}
	streamConvert(c, resp, translator.NewResponsesSseToChat("model"), false, "provider", "model", nil, "", "", time.Now())

	db, err := store.GetDB()
	if err != nil {
		t.Fatalf("get request db: %v", err)
	}
	var success int
	if err := db.QueryRow(`SELECT success FROM requests ORDER BY id DESC LIMIT 1`).Scan(&success); err != nil {
		t.Fatalf("read request row: %v", err)
	}
	return w, success == 1
}

func lastRequestSuccess(t *testing.T) bool {
	t.Helper()
	db, err := store.GetDB()
	if err != nil {
		t.Fatalf("get request db: %v", err)
	}
	var success int
	if err := db.QueryRow(`SELECT success FROM requests ORDER BY id DESC LIMIT 1`).Scan(&success); err != nil {
		t.Fatalf("read request row: %v", err)
	}
	return success == 1
}

func TestStreamConvert_CleanEOFIsSuccess(t *testing.T) {
	w, row := runConvertedStream(t,
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{}}\n\n",
		io.EOF,
	)
	if !row {
		t.Fatal("clean terminal EOF was recorded as failure")
	}
	if !strings.Contains(w.Body.String(), "data: [DONE]") {
		t.Fatalf("complete chat stream missing DONE marker: %s", w.Body.String())
	}
}

func TestStreamConvert_UnexpectedEOFIsFailure(t *testing.T) {
	w, row := runConvertedStream(t,
		"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n",
		io.ErrUnexpectedEOF,
	)
	if row {
		t.Fatal("unexpected EOF was recorded as success")
	}
	if strings.Contains(w.Body.String(), "data: [DONE]") {
		t.Fatalf("failed chat stream must not emit DONE: %s", w.Body.String())
	}
}

func TestStreamConvert_MissingTerminalIsFailure(t *testing.T) {
	_, row := runConvertedStream(t,
		"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n",
		io.EOF,
	)
	if row {
		t.Fatal("stream without a terminal event was recorded as success")
	}
}

func TestStreamConvert_ResponseFailedIsFailure(t *testing.T) {
	_, row := runConvertedStream(t,
		"event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"message\":\"boom\"}}}\n\n",
		io.EOF,
	)
	if row {
		t.Fatal("response.failed was recorded as success")
	}
}

func TestStreamPassthrough_UnexpectedEOFIsFailure(t *testing.T) {
	writeLegacyTestEnv(t, "")
	t.Cleanup(store.Close)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "http://proxy.test/v1/chat/completions", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       &scriptedBody{payload: []byte("data: {\"choices\":[]}\n\n"), err: io.ErrUnexpectedEOF},
		Request:    c.Request,
	}
	streamPassthrough(c, resp, translator.FormatOpenAIChat, "provider", "model", nil, "", "", time.Now())
	if lastRequestSuccess(t) {
		t.Fatal("unexpected EOF was recorded as success")
	}
}

func TestStreamPassthrough_ResponseFailedIsFailure(t *testing.T) {
	writeLegacyTestEnv(t, "")
	t.Cleanup(store.Close)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "http://proxy.test/v1/responses", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       &scriptedBody{payload: []byte("event: response.failed\ndata: {\"type\":\"response.failed\"}\n\n"), err: io.EOF},
		Request:    c.Request,
	}
	streamPassthrough(c, resp, translator.FormatOpenAIResponses, "provider", "model", nil, "", "", time.Now())
	if lastRequestSuccess(t) {
		t.Fatal("response.failed was recorded as success")
	}
}
