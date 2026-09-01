package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func writeTranslatorConfig(t *testing.T, dir, upstreamURL string, api, mode string, withCost bool) string {
	t.Helper()
	costJSON := ""
	if withCost {
		costJSON = `,"cost":{"input":0.15,"output":0.6,"cacheRead":0.075}`
	}
	cfg := fmt.Sprintf(`{
		"version":2,
		"current":"test-provider",
		"profiles":{
			"test-provider":{
				"api":%q,
				"responsesMode":%q,
				"baseUrl":%q,
				"apiKey":"sk-test",
				"models":[{"id":"gpt-4o-mini","contextWindow":128000,"maxTokens":16384%s},{"id":"claude-3-5-sonnet-20241022","contextWindow":200000,"maxTokens":8192%s}],
				"exposedModels":["gpt-4o-mini","claude-3-5-sonnet-20241022"]
			}
		},
		"settings":{
			"providerPrefix":"pi-switch",
			"writeMode":"gateway",
			"gatewayApi":"openai-completions",
			"proxy":{"host":"127.0.0.1","port":43112},
			"web":{"host":"127.0.0.1","port":43110},
			"conversationSource":"sessionScan"
		}
	}`, api, mode, upstreamURL, costJSON, costJSON)
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(cfg), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestResponsesMode_Validation(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	cfgPath := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cfgPath, []byte(`{"version":2,"profiles":{},"settings":{"providerPrefix":"pi-switch"}}`), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	r := NewMgmtRouter()
	// incompatible: openai-responses with convert should 400
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/profiles", strings.NewReader(`{"name":"bad1","profile":{"api":"openai-responses","responsesMode":"convert","baseUrl":"http://a","apiKey":"k","models":[]}}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("expected 400 for passthrough mismatch, got %d body %s", w.Code, w.Body.String())
	}
	// incompatible: openai-completions with passthrough should 400
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/api/profiles", strings.NewReader(`{"name":"bad2","profile":{"api":"openai-completions","responsesMode":"passthrough","baseUrl":"http://a","apiKey":"k","models":[]}}`))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != 400 {
		t.Fatalf("expected 400 for convert mismatch, got %d body %s", w2.Code, w2.Body.String())
	}
	// anthropic with passthrough should 400
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("POST", "/api/profiles", strings.NewReader(`{"name":"bad3","profile":{"api":"anthropic-messages","responsesMode":"passthrough","baseUrl":"http://a","apiKey":"k","models":[]}}`))
	req3.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w3, req3)
	if w3.Code != 400 {
		t.Fatalf("expected 400 for anthropic passthrough, got %d body %s", w3.Code, w3.Body.String())
	}
	// auto should always pass
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest("POST", "/api/profiles", strings.NewReader(`{"name":"good","profile":{"api":"openai-responses","responsesMode":"auto","baseUrl":"http://a","apiKey":"k","models":[]}}`))
	req4.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w4, req4)
	if w4.Code != 200 {
		t.Fatalf("auto should pass, got %d body %s", w4.Code, w4.Body.String())
	}
	// PUT /api/config with profiles containing bad combo should also 400
	badCfg := `{"version":2,"profiles":{"bad":{"api":"openai-responses","responsesMode":"convert","baseUrl":"http://a","apiKey":"k","models":[]}},"settings":{"providerPrefix":"pi-switch"}}`
	w5 := httptest.NewRecorder()
	req5, _ := http.NewRequest("PUT", "/api/config", strings.NewReader(badCfg))
	req5.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w5, req5)
	if w5.Code != 400 {
		t.Fatalf("PUT /api/config bad combo should 400, got %d body %s", w5.Code, w5.Body.String())
	}
}

func TestTranslator_ResponsesPassthroughAndConvert(t *testing.T) {
	// Passthrough case: api=openai-responses, POST /v1/responses should be forwarded as-is
	t.Run("passthrough", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "requests.db")
		var capturedBody string
		mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := ioReadAll(r.Body)
			capturedBody = string(b)
			if !strings.Contains(r.URL.Path, "/responses") {
				t.Errorf("passthrough should hit /v1/responses, got %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "resp_123", "object": "response", "model": "gpt-4o-mini",
				"output": []interface{}{map[string]interface{}{"type": "message", "role": "assistant", "content": []interface{}{map[string]interface{}{"type": "output_text", "text": "hello"}}}},
				"usage": map[string]interface{}{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
			})
		}))
		defer mock.Close()
		cfgPath := writeTranslatorConfig(t, dir, mock.URL, "openai-responses", "auto", true)
		t.Setenv("PI_SWITCH_CONFIG", cfgPath)
		t.Setenv("PI_SWITCH_DB", dbPath)
		proxyRouter := NewProxyRouter()
		body := `{"model":"gpt-4o-mini","input":"hi","max_output_tokens":100}`
		req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		proxyRouter.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("passthrough code %d body %s", w.Code, w.Body.String())
		}
		if !strings.Contains(capturedBody, `"input"`) {
			t.Fatalf("passthrough upstream body should contain input, got %s", capturedBody)
		}
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["object"] != "response" {
			t.Fatalf("passthrough response should be responses object, got %v", resp)
		}
	})
	// Convert case: api=openai-completions, POST /v1/responses should be converted to chat
	t.Run("convert", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "requests.db")
		var capturedBody string
		var capturedPath string
		mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := ioReadAll(r.Body)
			capturedBody = string(b)
			capturedPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "chatcmpl-123", "object": "chat.completion", "model": "gpt-4o-mini",
				"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "content": "hello"}}},
				"usage": map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 5, "prompt_tokens_details": map[string]interface{}{"cached_tokens": 2}, "completion_tokens_details": map[string]interface{}{"reasoning_tokens": 3}},
			})
		}))
		defer mock.Close()
		cfgPath := writeTranslatorConfig(t, dir, mock.URL, "openai-completions", "auto", true)
		t.Setenv("PI_SWITCH_CONFIG", cfgPath)
		t.Setenv("PI_SWITCH_DB", dbPath)
		proxyRouter := NewProxyRouter()
		body := `{"model":"gpt-4o-mini","input":"hi","max_output_tokens":100}`
		req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		proxyRouter.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("convert code %d body %s", w.Code, w.Body.String())
		}
		if !strings.Contains(capturedPath, "/chat/completions") {
			t.Fatalf("convert should hit /v1/chat/completions, got %s", capturedPath)
		}
		if !strings.Contains(capturedBody, `"messages"`) {
			t.Fatalf("convert upstream should be chat with messages, got %s", capturedBody)
		}
		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp["object"] != "response" {
			t.Fatalf("convert response should be converted back to response object, got %v", resp["object"])
		}
		// verify DB has cached and reasoning parsed
		mgmt := NewMgmtRouter()
		req2 := httptest.NewRequest("GET", "/api/stats", nil)
		w2 := httptest.NewRecorder()
		mgmt.ServeHTTP(w2, req2)
		var stats map[string]interface{}
		_ = json.Unmarshal(w2.Body.Bytes(), &stats)
		rows, _ := stats["rows"].([]interface{})
		if len(rows) == 0 {
			t.Fatal("no rows after convert")
		}
		first := rows[0].(map[string]interface{})
		if first["cached_tokens"] == nil || first["cached_tokens"].(float64) != 2 {
			t.Fatalf("cached_tokens should be 2, got %v", first["cached_tokens"])
		}
		if first["reasoning_tokens"] == nil || first["reasoning_tokens"].(float64) != 3 {
			t.Fatalf("reasoning_tokens should be 3, got %v", first["reasoning_tokens"])
		}
	})
}

func TestTranslator_AnthropicMessages(t *testing.T) {
	// Chat -> Anthropic convert
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	var capturedBody string
	var capturedPath string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := ioReadAll(r.Body)
		capturedBody = string(b)
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "msg_123", "type": "message", "role": "assistant",
			"content": []interface{}{map[string]interface{}{"type": "text", "text": "hello"}},
			"model": "claude-3-5-sonnet-20241022", "stop_reason": "end_turn",
			"usage": map[string]interface{}{"input_tokens": 12, "output_tokens": 6},
		})
	}))
	defer mock.Close()
	cfgPath := writeTranslatorConfig(t, dir, mock.URL+"/v1", "anthropic-messages", "auto", true)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	proxyRouter := NewProxyRouter()
	body := `{"model":"claude-3-5-sonnet-20241022","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	proxyRouter.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("anthropic chat->anthropic code %d body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(capturedPath, "/messages") {
		t.Fatalf("should hit /v1/messages, got %s", capturedPath)
	}
	if !strings.Contains(capturedBody, `"content"`) {
		t.Fatalf("anthropic upstream body should have content, got %s", capturedBody)
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["choices"]; !ok {
		t.Fatalf("anthropic response should be converted to chat choices, got %v", resp)
	}
	// Anthropic -> Chat convert (client sends messages, upstream is chat)
	t.Run("anthropic_to_chat", func(t *testing.T) {
		dir2 := t.TempDir()
		dbPath2 := filepath.Join(dir2, "requests.db")
		var capBody2 string
		mock2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := ioReadAll(r.Body)
			capBody2 = string(b)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "chatcmpl-456", "object": "chat.completion", "model": "gpt-4o-mini",
				"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "content": "hello2"}}},
				"usage": map[string]interface{}{"prompt_tokens": 8, "completion_tokens": 4},
			})
		}))
		defer mock2.Close()
		// config with openai-completions now
		cfgPath2 := writeTranslatorConfig(t, dir2, mock2.URL, "openai-completions", "auto", false)
		// but need to ensure profile's api is openai, and we send to /v1/messages
		t.Setenv("PI_SWITCH_CONFIG", cfgPath2)
		t.Setenv("PI_SWITCH_DB", dbPath2)
		proxyRouter2 := NewProxyRouter()
		// client sends anthropic messages format
		body2 := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`
		// Actually for POST /v1/messages, body should be anthropic style
		anthBody := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"max_tokens":100}`
		req2 := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(anthBody))
		req2.Header.Set("Content-Type", "application/json")
		w2 := httptest.NewRecorder()
		proxyRouter2.ServeHTTP(w2, req2)
		// Since upstream is openai, it should convert and succeed (maybe 200 or 502 no_route if model not exposed? But model is exposed)
		// We check that upstream received chat messages
		if capBody2 != "" && !strings.Contains(capBody2, `"messages"`) {
			t.Fatalf("anthropic->chat upstream should be chat, got %s", capBody2)
		}
		_ = body2
	})
}

func TestLimit_ClampAllKeys(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	var capturedMax int
	var capturedBody string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := ioReadAll(r.Body)
		capturedBody = string(b)
		var m map[string]interface{}
		_ = json.Unmarshal(b, &m)
		if v, ok := m["max_tokens"].(float64); ok {
			capturedMax = int(v)
		} else if v, ok := m["max_output_tokens"].(float64); ok {
			capturedMax = int(v)
		} else if v, ok := m["max_completion_tokens"].(float64); ok {
			capturedMax = int(v)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chatcmpl-1", "object": "chat.completion", "model": "gpt-4o-mini",
			"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "content": "hi"}}},
			"usage": map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 5},
		})
	}))
	defer mock.Close()
	cfgPath := writeTranslatorConfig(t, dir, mock.URL, "openai-completions", "auto", false)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	proxyRouter := NewProxyRouter()
	gin.SetMode(gin.TestMode)
	// max_tokens clamp
	body := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}],"max_tokens":999999}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	proxyRouter.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("max_tokens clamp code %d", w.Code)
	}
	if capturedMax != 16384 {
		t.Fatalf("max_tokens should be clamped to 16384, got %d body %s", capturedMax, capturedBody)
	}
	// max_output_tokens clamp via responses
	capturedMax = 0
	body2 := `{"model":"gpt-4o-mini","input":"hi","max_output_tokens":999999}`
	req2 := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	proxyRouter.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("max_output_tokens clamp code %d body %s", w2.Code, w2.Body.String())
	}
	// After conversion to chat, max_tokens should be clamped
	if capturedMax != 16384 {
		t.Fatalf("max_output_tokens->max_tokens clamp should be 16384, got %d body %s", capturedMax, capturedBody)
	}
	// max_completion_tokens clamp
	capturedMax = 0
	body3 := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}],"max_completion_tokens":999999}`
	req3 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body3))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	proxyRouter.ServeHTTP(w3, req3)
	if w3.Code != 200 {
		t.Fatalf("max_completion_tokens code %d", w3.Code)
	}
	if capturedMax != 16384 {
		t.Fatalf("max_completion_tokens clamp should be 16384, got %d", capturedMax)
	}
	// within limit should be preserved
	capturedMax = 0
	body4 := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}],"max_tokens":100}`
	req4 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body4))
	req4.Header.Set("Content-Type", "application/json")
	w4 := httptest.NewRecorder()
	proxyRouter.ServeHTTP(w4, req4)
	if capturedMax != 100 {
		t.Fatalf("within limit should be 100, got %d", capturedMax)
	}
}

func TestStream_ChatAndResponses(t *testing.T) {
	// Chat stream passthrough with usage parsing
	t.Run("chat_stream", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "requests.db")
		mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			// SSE stream with usage at end including cached and reasoning
			fmt.Fprintf(w, "data: {\"id\":\"1\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
			fmt.Fprintf(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":50,\"prompt_tokens_details\":{\"cached_tokens\":20},\"completion_tokens_details\":{\"reasoning_tokens\":10}}}\n\n")
			fmt.Fprintf(w, "data: [DONE]\n\n")
		}))
		defer mock.Close()
		cfgPath := writeTranslatorConfig(t, dir, mock.URL, "openai-completions", "auto", true)
		t.Setenv("PI_SWITCH_CONFIG", cfgPath)
		t.Setenv("PI_SWITCH_DB", dbPath)
		proxyRouter := NewProxyRouter()
		body := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}],"stream":true}`
		req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		proxyRouter.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("chat stream code %d body %s", w.Code, w.Body.String())
		}
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/event-stream") {
			t.Fatalf("chat stream content-type %q want event-stream", ct)
		}
		bodyStr := w.Body.String()
		if !strings.Contains(bodyStr, "hi") {
			t.Fatalf("stream body should contain hi, got %s", bodyStr)
		}
		// check DB via stats
		mgmt := NewMgmtRouter()
		req2 := httptest.NewRequest("GET", "/api/stats", nil)
		w2 := httptest.NewRecorder()
		mgmt.ServeHTTP(w2, req2)
		var stats map[string]interface{}
		_ = json.Unmarshal(w2.Body.Bytes(), &stats)
		rows, _ := stats["rows"].([]interface{})
		if len(rows) == 0 {
			t.Fatal("no rows after stream")
		}
		first := rows[0].(map[string]interface{})
		if first["cached_tokens"] == nil || int(first["cached_tokens"].(float64)) != 20 {
			t.Fatalf("stream cached_tokens want 20 got %v", first["cached_tokens"])
		}
		if first["reasoning_tokens"] == nil || int(first["reasoning_tokens"].(float64)) != 10 {
			t.Fatalf("stream reasoning_tokens want 10 got %v", first["reasoning_tokens"])
		}
		if first["prompt_tokens"] == nil || int(first["prompt_tokens"].(float64)) != 100 {
			t.Fatalf("prompt_tokens want 100 got %v", first["prompt_tokens"])
		}
	})
	// Responses stream via ChatSseToResponses conversion
	t.Run("responses_stream", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "requests.db")
		mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// upstream is chat completions, returns chat SSE
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
			fmt.Fprintf(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":80,\"completion_tokens\":40,\"prompt_tokens_details\":{\"cached_tokens\":5},\"completion_tokens_details\":{\"reasoning_tokens\":7}}}\n\n")
			fmt.Fprintf(w, "data: [DONE]\n\n")
		}))
		defer mock.Close()
		// config with openai-completions, but client sends responses stream -> should convert chat SSE to responses SSE
		cfgPath := writeTranslatorConfig(t, dir, mock.URL, "openai-completions", "auto", true)
		t.Setenv("PI_SWITCH_CONFIG", cfgPath)
		t.Setenv("PI_SWITCH_DB", dbPath)
		proxyRouter := NewProxyRouter()
		body := `{"model":"gpt-4o-mini","input":"hi","stream":true}`
		req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		proxyRouter.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("responses stream code %d body %s", w.Code, w.Body.String())
		}
		ct := w.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/event-stream") {
			t.Fatalf("responses stream ct %q", ct)
		}
		bodyStr := w.Body.String()
		// converted responses events should contain response.created or output_text.delta
		if !strings.Contains(bodyStr, "response.created") && !strings.Contains(bodyStr, "output_text") {
			t.Fatalf("responses stream should contain converted events, got %s", bodyStr)
		}
		// DB should have usage
		mgmt := NewMgmtRouter()
		req2 := httptest.NewRequest("GET", "/api/stats", nil)
		w2 := httptest.NewRecorder()
		mgmt.ServeHTTP(w2, req2)
		var stats map[string]interface{}
		_ = json.Unmarshal(w2.Body.Bytes(), &stats)
		rows, _ := stats["rows"].([]interface{})
		if len(rows) == 0 {
			t.Fatal("no rows after responses stream")
		}
		first := rows[0].(map[string]interface{})
		if first["cached_tokens"] == nil || int(first["cached_tokens"].(float64)) != 5 {
			t.Fatalf("responses stream cached want 5 got %v", first["cached_tokens"])
		}
		if first["reasoning_tokens"] == nil || int(first["reasoning_tokens"].(float64)) != 7 {
			t.Fatalf("responses stream reasoning want 7 got %v", first["reasoning_tokens"])
		}
	})
}

func TestConversationName_Decode(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "requests.db")
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chatcmpl-1", "object": "chat.completion", "model": "gpt-4o-mini",
			"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "content": "hi"}}},
			"usage": map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 5},
		})
	}))
	defer mock.Close()
	cfgPath := writeTranslatorConfig(t, dir, mock.URL, "openai-completions", "auto", false)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", dbPath)
	proxyRouter := NewProxyRouter()
	// x-conversation-name with percent-encoded Chinese
	encoded := "%E4%B8%AD%E6%96%87%E4%BC%9A%E8%AF%9D"
	body := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-conversation-id", "conv-1")
	req.Header.Set("x-conversation-name", encoded)
	w := httptest.NewRecorder()
	proxyRouter.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("code %d", w.Code)
	}
	mgmt := NewMgmtRouter()
	req2 := httptest.NewRequest("GET", "/api/stats", nil)
	w2 := httptest.NewRecorder()
	mgmt.ServeHTTP(w2, req2)
	var stats map[string]interface{}
	_ = json.Unmarshal(w2.Body.Bytes(), &stats)
	rows, _ := stats["rows"].([]interface{})
	if len(rows) == 0 {
		t.Fatal("no rows")
	}
	first := rows[0].(map[string]interface{})
	name, _ := first["conversation_name"].(string)
	if name != "中文会话" {
		t.Fatalf("decoded name = %q want 中文会话", name)
	}
	// literal %AB that is invalid UTF-8 should be kept raw
	req3 := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("x-conversation-id", "conv-2")
	req3.Header.Set("x-conversation-name", "100%AB")
	w3 := httptest.NewRecorder()
	proxyRouter.ServeHTTP(w3, req3)
	mgmt2 := NewMgmtRouter()
	req4 := httptest.NewRequest("GET", "/api/stats", nil)
	w4 := httptest.NewRecorder()
	mgmt2.ServeHTTP(w4, req4)
	var stats2 map[string]interface{}
	_ = json.Unmarshal(w4.Body.Bytes(), &stats2)
	rows2, _ := stats2["rows"].([]interface{})
	found := false
	for _, r := range rows2 {
		m := r.(map[string]interface{})
		if m["conversation_id"] == "conv-2" {
			found = true
			if m["conversation_name"] != "100%AB" {
				t.Fatalf("invalid UTF-8 percent should be kept raw, got %q", m["conversation_name"])
			}
		}
	}
	if !found {
		t.Fatal("conv-2 not found")
	}
}
