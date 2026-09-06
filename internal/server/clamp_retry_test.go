package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetryOn400ToMin(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG", filepath.Join(dir, "config.json"))
	resetRetryStateForTest()

	var callCount int
	var lastBody string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		b, _ := io.ReadAll(r.Body)
		lastBody = string(b)
		if callCount == 1 {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"type":    "invalid_request_error",
				"message": "Error from provider (Console Go): Upstream request failed: [invalid_request_error] The request contains invalid parameters. Check the request body",
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chatcmpl-ok", "object": "chat.completion", "model": "muse-spark-1.2-contributor",
			"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "content": "ok"}}},
			"usage": map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 5},
		})
	}))
	defer mock.Close()

	cfg := fmt.Sprintf(`{
		"version":2,
		"current":"oc",
		"profiles":{
			"oc":{"api":"openai-responses","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-oc","models":[{"id":"muse-spark-1.2-contributor","contextWindow":1048576,"maxTokens":943718}],"exposedModels":["muse-spark-1.2-contributor"]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","conversationSource":"off","proxy":{"host":"127.0.0.1","port":43112,"circuitBreaker":{"enabled":true,"failureThreshold":3,"cooldownSeconds":60}},"web":{"host":"127.0.0.1","port":43110}}
	}`, mock.URL+"/v1")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	router := NewProxyRouter()
	body := `{"model":"oc/muse-spark-1.2-contributor","input":"hi","max_output_tokens":908720}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected retry to succeed with 200, got %d body %s callCount=%d lastBody=%s", w.Code, w.Body.String(), callCount, lastBody)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 calls (retry), got %d", callCount)
	}
	if !strings.Contains(lastBody, `"max_output_tokens":16`) && !strings.Contains(lastBody, `"max_output_tokens": 16`) {
		t.Fatalf("second retry should have max_output_tokens=16, got %s", lastBody)
	}
}
