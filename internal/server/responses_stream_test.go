package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// RED: chat client against a Responses upstream must get Chat SSE back,
// not raw Responses SSE (dispatch must come from the translator registry).
func TestStream_ResponsesUpstreamToChat(t *testing.T) {
	resetRetryStateForTest()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	var capturedPath string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n")
		fmt.Fprintf(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"hello\"}\n\n")
		fmt.Fprintf(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":8,\"output_tokens\":4,\"total_tokens\":12}}}\n\n")
	}))
	defer mock.Close()
	cfgPath := writeTranslatorConfig(t, dir, mock.URL, "openai-responses", "auto", true)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	proxyRouter := NewProxyRouter()
	body := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}],"stream":true}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	proxyRouter.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("code %d body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(capturedPath, "/responses") {
		t.Fatalf("upstream should be /v1/responses, got %s", capturedPath)
	}
	got := w.Body.String()
	if strings.Contains(got, "response.output_text.delta") {
		t.Fatalf("downstream must be Chat SSE, got raw Responses events: %s", got)
	}
	if !strings.Contains(got, `"content":"hello"`) {
		t.Fatalf("downstream should carry chat delta hello, got %s", got)
	}
	if !strings.Contains(got, "data: [DONE]") {
		t.Fatalf("chat stream must end with data: [DONE], got %s", got)
	}
	// usage lands in stats via the converted usage
	mgmt := NewMgmtRouter()
	req2 := httptest.NewRequest("GET", "/api/stats", nil)
	w2 := httptest.NewRecorder()
	mgmt.ServeHTTP(w2, req2)
	var stats map[string]interface{}
	_ = json.Unmarshal(w2.Body.Bytes(), &stats)
	rows, _ := stats["rows"].([]interface{})
	if len(rows) == 0 {
		t.Fatal("no rows after responses->chat stream")
	}
	first := rows[0].(map[string]interface{})
	if first["prompt_tokens"] == nil || int(first["prompt_tokens"].(float64)) != 8 {
		t.Fatalf("prompt_tokens want 8 got %v", first["prompt_tokens"])
	}
}

func TestStream_ResponsesIncompleteToChatLength(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
		fmt.Fprint(w, "event: response.incomplete\ndata: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}\n\n")
	}))
	defer mock.Close()
	cfgPath := writeTranslatorConfig(t, dir, mock.URL, "openai-responses", "auto", false)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	router := NewProxyRouter()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code %d body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"finish_reason":"length"`) {
		t.Fatalf("incomplete Responses stream must map to Chat length finish: %s", w.Body.String())
	}
}

func TestStream_ResponsesFailureToChatError(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"message\":\"upstream boom\"}}}\n\n")
	}))
	defer mock.Close()
	cfgPath := writeTranslatorConfig(t, dir, mock.URL, "openai-responses", "auto", false)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	router := NewProxyRouter()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code %d body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"error":{"message":"upstream boom"`) {
		t.Fatalf("failure stream must preserve upstream error: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"finish_reason":"stop"`) {
		t.Fatalf("failure stream must not emit successful stop finish: %s", w.Body.String())
	}
}
