package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func channelAPIMock(t *testing.T, expectedPath string, okBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":"unexpected path ` + r.URL.Path + ` want ` + expectedPath + `"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(okBody))
	}))
}

func newOCConfig(chatURL, respURL string) string {
	return `{"version":2,"current":"oc","profiles":{"oc":{"api":"openai-completions","responsesMode":"auto","upstreams":[
		{"name":"chat","baseUrl":` + jsonString(chatURL) + `,"apiKey":"k1","api":"openai-completions","responsesMode":"auto","models":[{"id":"mimo-v2.5","contextWindow":128000,"maxTokens":16384}],"exposedModels":["mimo-v2.5"]},
		{"name":"responses","baseUrl":` + jsonString(respURL) + `,"apiKey":"k2","api":"openai-responses","responsesMode":"auto","models":[{"id":"muse-spark-1.2-contributor","contextWindow":128000,"maxTokens":16384}],"exposedModels":["muse-spark-1.2-contributor"]}]}},
		"settings":{"providerPrefix":"pi-switch","conversationSource":"off"}}`
}

func TestChannelAPI_RoutingDirect(t *testing.T) {
	chatOK := `{"id":"c1","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"}}],"model":"mimo-v2.5","usage":{"prompt_tokens":1,"completion_tokens":1}}`
	respOK := `{"id":"resp1","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"model":"muse-spark-1.2-contributor","usage":{"input_tokens":1,"output_tokens":1}}`
	chatMock := channelAPIMock(t, "/v1/chat/completions", chatOK)
	defer chatMock.Close()
	respMock := channelAPIMock(t, "/v1/responses", respOK)
	defer respMock.Close()

	dir := t.TempDir()
	writeChannelConfig(t, dir, newOCConfig(chatMock.URL, respMock.URL))

	// chat via bare mimo -> 200 (chat path on chat channel, bare id)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"mimo-v2.5","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	NewProxyRouter().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("chat direct mimo code=%d body=%s, want 200", w.Code, w.Body.String())
	}

	// responses via bare muse -> 200 (responses path on responses channel, bare id)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"muse-spark-1.2-contributor","input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`))
	req2.Header.Set("Content-Type", "application/json")
	NewProxyRouter().ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("responses direct muse-spark code=%d body=%s, want 200", w2.Code, w2.Body.String())
	}
}

func TestChannelAPI_CrossConversion(t *testing.T) {
	chatOK := `{"id":"c1","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"}}],"model":"mimo-v2.5","usage":{"prompt_tokens":1,"completion_tokens":1}}`
	respOK := `{"id":"resp1","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"model":"muse-spark-1.2-contributor","usage":{"input_tokens":1,"output_tokens":1}}`
	chatMock := channelAPIMock(t, "/v1/chat/completions", chatOK)
	defer chatMock.Close()
	respMock := channelAPIMock(t, "/v1/responses", respOK)
	defer respMock.Close()

	dir := t.TempDir()
	writeChannelConfig(t, dir, newOCConfig(chatMock.URL, respMock.URL))

	// cross: chat proto via responses channel (bare muse) -> convert chat->responses, hit /v1/responses 200
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"muse-spark-1.2-contributor","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	NewProxyRouter().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("cross chat->responses muse-spark code=%d body=%s, want 200", w.Code, w.Body.String())
	}

	// cross: responses proto via chat channel (bare mimo) -> convert responses->chat, hit /v1/chat/completions 200
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"mimo-v2.5","input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`))
	req2.Header.Set("Content-Type", "application/json")
	NewProxyRouter().ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("cross responses->chat mimo code=%d body=%s, want 200", w2.Code, w2.Body.String())
	}
}

func TestChannelAPI_ModelsList(t *testing.T) {
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
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := resp["data"].([]interface{})
	ids := map[string]string{}
	owned := map[string]string{}
	for _, m := range data {
		mm := m.(map[string]interface{})
		ids[mm["id"].(string)] = mm["owned_by"].(string)
		owned[mm["id"].(string)] = mm["owned_by"].(string)
	}
	if _, ok := ids["mimo-v2.5"]; !ok {
		t.Fatalf("models ids=%v want mimo-v2.5", ids)
	}
	if _, ok := ids["muse-spark-1.2-contributor"]; !ok {
		t.Fatalf("models ids=%v want muse-spark-1.2-contributor", ids)
	}
	// owned_by should be per-channel providerKey
	if owned["mimo-v2.5"] != "oc/chat" {
		t.Fatalf("mimo owned_by=%q want oc/chat", owned["mimo-v2.5"])
	}
	if owned["muse-spark-1.2-contributor"] != "oc/responses" {
		t.Fatalf("muse owned_by=%q want oc/responses", owned["muse-spark-1.2-contributor"])
	}
}
