package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSlashModelIDsAreRejected(t *testing.T) {
	chatOK := `{"id":"c1","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"}}],"model":"mimo-v2.5","usage":{"prompt_tokens":1,"completion_tokens":1}}`
	respOK := `{"id":"resp1","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"model":"muse-spark-1.2-contributor","usage":{"input_tokens":1,"output_tokens":1}}`
	chatMock := channelAPIMock(t, "/v1/chat/completions", chatOK)
	defer chatMock.Close()
	respMock := channelAPIMock(t, "/v1/responses", respOK)
	defer respMock.Close()

	dir := t.TempDir()
	writeChannelConfig(t, dir, newOCConfig(chatMock.URL, respMock.URL))

	// Provider/channel prefixes are no longer accepted at the proxy boundary.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"oc/mimo-v2.5","input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	NewProxyRouter().ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("slash model id code=%d body=%s want 400", w.Code, w.Body.String())
	}

	// The same strict rule applies to every endpoint.
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"oc/muse-spark-1.2-contributor","messages":[{"role":"user","content":"hi"}]}`))
	req2.Header.Set("Content-Type", "application/json")
	NewProxyRouter().ServeHTTP(w2, req2)
	if w2.Code != 400 {
		t.Fatalf("slash model id code=%d body=%s want 400", w2.Code, w2.Body.String())
	}
}

func TestShortAlias_ModelsListContainsShort(t *testing.T) {
	chatMock := channelAPIMock(t, "/v1/chat/completions", `{"id":"c","object":"chat.completion","choices":[]}`)
	defer chatMock.Close()
	respMock := channelAPIMock(t, "/v1/responses", `{"id":"r","object":"response","status":"completed","output":[]}`)
	defer respMock.Close()

	dir := t.TempDir()
	writeChannelConfig(t, dir, newOCConfig(chatMock.URL, respMock.URL))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/models", nil)
	NewProxyRouter().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("models list code=%d", w.Code)
	}
	body := w.Body.String()
	// New per-channel bare: no short alias, only bare ids with owned_by
	if strings.Contains(body, `"id":"oc/mimo-v2.5"`) {
		t.Fatalf("models should not contain short alias oc/mimo-v2.5 after bare migration, got %s", body)
	}
	if !strings.Contains(body, `"id":"mimo-v2.5"`) {
		t.Fatalf("models should contain bare mimo-v2.5, got %s", body)
	}
	if !strings.Contains(body, `"id":"muse-spark-1.2-contributor"`) {
		t.Fatalf("models should contain bare muse, got %s", body)
	}
	if !strings.Contains(body, `"owned_by":"oc/chat"`) || !strings.Contains(body, `"owned_by":"oc/responses"`) {
		t.Fatalf("models should have owned_by per channel, got %s", body)
	}
}

func TestShortAlias_Ambiguous(t *testing.T) {
	chatOK := `{"id":"c1","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"}}],"model":"dup","usage":{"prompt_tokens":1,"completion_tokens":1}}`
	chatMock := channelAPIMock(t, "/v1/chat/completions", chatOK)
	defer chatMock.Close()
	respMock := channelAPIMock(t, "/v1/responses", `{"id":"r","object":"response","status":"completed","output":[]}`)
	defer respMock.Close()

	dupConfig := `{"version":2,"current":"oc","profiles":{"oc":{"api":"openai-completions","responsesMode":"auto","upstreams":[
		{"name":"chat","baseUrl":"` + chatMock.URL + `","apiKey":"k1","api":"openai-completions","responsesMode":"auto","models":[{"id":"dup","contextWindow":128000,"maxTokens":16384}],"exposedModels":["dup"]},
		{"name":"responses","baseUrl":"` + respMock.URL + `","apiKey":"k2","api":"openai-responses","responsesMode":"auto","models":[{"id":"dup","contextWindow":128000,"maxTokens":16384}],"exposedModels":["dup"]}]}},
		"settings":{"providerPrefix":"pi-switch","conversationSource":"off"}}`
	dir := t.TempDir()
	writeChannelConfig(t, dir, dupConfig)

	// After bare migration, oc/dup (with slash) should be 400, bare dup should be 502 ambiguous
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"oc/dup","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	NewProxyRouter().ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("oc/dup with slash should be 400, got %d body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid_request_error") {
		t.Fatalf("slash error should contain invalid_request_error, got %s", w.Body.String())
	}
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"dup","messages":[{"role":"user","content":"hi"}]}`))
	req3.Header.Set("Content-Type", "application/json")
	NewProxyRouter().ServeHTTP(w3, req3)
	if w3.Code != 502 {
		t.Fatalf("ambiguous bare dup code=%d body=%s want 502", w3.Code, w3.Body.String())
	}
	if !strings.Contains(w3.Body.String(), "ambiguous") {
		t.Fatalf("ambiguous error should contain 'ambiguous', got %s", w3.Body.String())
	}

	// models list should have bare dup with two providers, no short alias
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/v1/models", nil)
	NewProxyRouter().ServeHTTP(w2, req2)
	body := w2.Body.String()
	if strings.Contains(body, `"id":"oc/dup"`) {
		t.Fatalf("ambiguous dup should not expose short alias oc/dup, got %s", body)
	}
	// Should have bare dup twice with different owned_by
	if !strings.Contains(body, `"id":"dup"`) {
		t.Fatalf("bare dup should exist, got %s", body)
	}
	if !strings.Contains(body, `"owned_by":"oc/chat"`) || !strings.Contains(body, `"owned_by":"oc/responses"`) {
		t.Fatalf("bare dup should have both channels, got %s", body)
	}
	// Full 3-segment should not exist after bare migration
	if strings.Contains(body, "oc/chat/dup") && strings.Contains(body, `"id":"oc/chat/dup"`) {
		t.Fatalf("should not have prefixed 3-segment after bare, got %s", body)
	}
}
