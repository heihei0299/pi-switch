package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

// 额度查询回归：渠道分区后顶层 Models 为空，模型只在各 channel 池里。
// handleGetCredits 必须看渠道池并真实回源 /usage，而不是返回全零或写死假数据。
func TestCredits_PartitionedChannelPoolFetchesUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotUA string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.UserAgent()
		if r.URL.Path != "/usage" {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"usage":{"rolling":{"status":"ok","percent":4.5,"resetsAt":"2026-09-07T06:31:08Z"},"weekly":{"status":"ok","percent":1},"monthly":{"status":"ok","percent":77}}}`))
	}))
	defer mock.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfgContent := fmt.Sprintf(`{"version":2,"profiles":{"oc":{"api":"openai-completions","responsesMode":"auto","upstreams":[{"name":"chat","baseUrl":%q,"apiKey":"k","api":"openai-completions","responsesMode":"auto","models":[{"id":"mimo-v2.5","contextWindow":128000,"maxTokens":16384}],"exposedModels":["mimo-v2.5"]}]}},"settings":{"providerPrefix":"pi-switch"}}`, mock.URL)
	_ = os.WriteFile(cfgPath, []byte(cfgContent), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)

	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/profiles/oc/credits", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("partitioned credits code %d body %s, want 200 with real usage", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	usage, _ := resp["usage"].(map[string]interface{})
	rolling, _ := usage["rolling"].(map[string]interface{})
	if rp, _ := rolling["percent"].(float64); rp != 4.5 {
		t.Fatalf("rolling percent = %v want 4.5 (upstream value): %v", rp, resp)
	}
	if gotUA == "" {
		t.Fatalf("upstream request must carry User-Agent (Cloudflare blocks UA-less requests)")
	}
}

func TestCredits_Upstream403SurfacesError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"EntitlementError","message":"forbidden"}}`))
	}))
	defer mock.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfgContent := fmt.Sprintf(`{"version":2,"profiles":{"op":{"api":"openai-completions","baseUrl":%q,"apiKey":"k","upstreams":[{"name":"main","api":"openai-completions","baseUrl":%q,"apiKey":"k","models":[{"id":"m1","contextWindow":128000,"maxTokens":16384}]}]}},"settings":{"providerPrefix":"pi-switch"}}`, mock.URL, mock.URL)
	_ = os.WriteFile(cfgPath, []byte(cfgContent), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)

	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/profiles/op/credits", nil)
	r.ServeHTTP(w, req)
	if w.Code == 200 {
		t.Fatalf("upstream 403 must not return 200 (stub/fake numbers forbidden): %s", w.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["error"]; !ok {
		t.Fatalf("error response must carry error field: %v", resp)
	}
}

func TestCredits_NoModelsAnywhereReturnsZeros(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	cfgContent := `{"version":2,"profiles":{"empty":{"api":"openai-completions","baseUrl":"https://api.example.com/v1","apiKey":"k","models":[],"proxy":false}},"settings":{"providerPrefix":"pi-switch"}}`
	_ = os.WriteFile(cfgPath, []byte(cfgContent), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)

	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/profiles/empty/credits", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("empty credits code %d", w.Code)
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["percent"] == nil {
		t.Fatalf("percent must be number 0, got nil: %v", resp)
	}
}

func TestCredits_UnknownProfile404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cfgPath, []byte(`{"version":2,"profiles":{},"settings":{"providerPrefix":"pi-switch"}}`), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)

	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/profiles/nope/credits", nil)
	r.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Fatalf("unknown profile credits code %d want 404", w.Code)
	}
}
