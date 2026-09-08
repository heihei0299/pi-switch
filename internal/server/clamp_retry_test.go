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
	var affinityHeaders []string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		affinityHeaders = append(affinityHeaders, r.Header.Get("x-opencode-session"))
		b, _ := io.ReadAll(r.Body)
		lastBody = string(b)
		if r.Header.Get("x-opencode-session") != "session-123" {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"type":    "invalid_request_error",
				"message": "MissingSessionID",
			})
			return
		}
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
			"usage":   map[string]interface{}{"prompt_tokens": 10, "completion_tokens": 5},
		})
	}))
	defer mock.Close()
	// The userinfo keeps the request local while exercising the opencode.ai affinity branch.
	opencodeBase := strings.Replace(mock.URL, "http://", "http://opencode.ai@", 1) + "/v1"

	cfg := fmt.Sprintf(`{
		"version":2,
		"current":"oc",
		"profiles":{
			"oc":{"api":"openai-responses","responsesMode":"auto","baseUrl":%q,"apiKey":"sk-oc","upstreams":[{"name":"main","api":"openai-responses","baseUrl":%q,"apiKey":"sk-oc","models":[{"id":"muse-spark-1.2-contributor","contextWindow":1048576,"maxTokens":943718}],"exposedModels":["muse-spark-1.2-contributor"]}]}
		},
		"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","conversationSource":"off","proxy":{"host":"127.0.0.1","port":43112,"circuitBreaker":{"enabled":true,"failureThreshold":3,"cooldownSeconds":60}},"web":{"host":"127.0.0.1","port":43110}}
	}`, opencodeBase, opencodeBase)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	router := NewProxyRouter()
	body := `{"model":"muse-spark-1.2-contributor","input":"hi","max_output_tokens":908720}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-opencode-session", "session-123")
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected retry to succeed with 200, got %d body %s callCount=%d lastBody=%s affinity=%q", w.Code, w.Body.String(), callCount, lastBody, affinityHeaders)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 calls (retry), got %d", callCount)
	}
	if len(affinityHeaders) != 2 || affinityHeaders[0] != "session-123" || affinityHeaders[1] != "session-123" {
		t.Fatalf("both attempts must preserve OpenCode affinity, got %q", affinityHeaders)
	}
	if !strings.Contains(lastBody, `"max_output_tokens":16`) && !strings.Contains(lastBody, `"max_output_tokens": 16`) {
		t.Fatalf("second retry should have max_output_tokens=16, got %s", lastBody)
	}
}
