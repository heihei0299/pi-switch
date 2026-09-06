package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// S2: 渠道 name 主键校验（必填、同一供应商内唯一、1-32 字符、仅字母数字/-/_），
// 经 POST /api/profiles 可观测：非法 400，合法 200。

func postProfile(t *testing.T, payload string) int {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cfgPath, []byte(`{"version":2,"profiles":{},"settings":{"providerPrefix":"pi-switch"}}`), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/profiles", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w.Code
}

func TestChannelValidation_NameRules(t *testing.T) {
	base := `{"name":"p%d","profile":{"api":"openai-completions","responsesMode":"auto","baseUrl":"http://a","apiKey":"k","models":[]%s}}`
	cases := []struct {
		name      string
		upstreams string
		want      int
	}{
		{"missing name", `,"upstreams":[{"baseUrl":"http://a","apiKey":"k"}]`, 400},
		{"empty name", `,"upstreams":[{"name":"","baseUrl":"http://a","apiKey":"k"}]`, 400},
		{"duplicate name", `,"upstreams":[{"name":"c1","baseUrl":"http://a","apiKey":"k"},{"name":"c1","baseUrl":"http://b","apiKey":"k"}]`, 400},
		{"space in name", `,"upstreams":[{"name":"bad name","baseUrl":"http://a","apiKey":"k"}]`, 400},
		{"too long", `,"upstreams":[{"name":"123456789012345678901234567890123","baseUrl":"http://a","apiKey":"k"}]`, 400},
		{"valid pair", `,"upstreams":[{"name":"main","baseUrl":"http://a","apiKey":"k"},{"name":"bk-2_x","baseUrl":"http://b","apiKey":"k"}]`, 200},
		{"no upstreams", ``, 200},
	}
	for i, tc := range cases {
		if code := postProfile(t, fmt.Sprintf(base, i, tc.upstreams)); code != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.name, code, tc.want)
		}
	}
}
