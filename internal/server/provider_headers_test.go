package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/store"
)

func TestProviderCommandsAndProxyShareConfiguredHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Tenant") != "channel" || r.Header.Get("X-Account") != "account" || r.Header.Get("Authorization") != "channel-auth" {
			w.WriteHeader(403)
			fmt.Fprint(w, "missing configured headers")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"data":[{"id":"new-model"}]}`)
		} else {
			fmt.Fprint(w, `{"choices":[{"message":{"content":"hello"}}]}`)
		}
	}))
	defer upstream.Close()
	auditFixture(t, "openai-completions", upstream.URL)
	if err := config.UpdateAtPath(config.ResolvePath(), func(cfg *config.PiSwitchConfig) error {
		p := cfg.Profiles["audit"]
		p.BaseURL, p.APIKey = upstream.URL, "automatic-key"
		p.Headers = map[string]string{"X-Tenant": "profile", "X-Account": "account", "Authorization": "profile-auth"}
		p.Upstreams[0].BaseURL = ""
		p.Upstreams[0].Headers = map[string]string{"x-tenant": "channel", "authorization": "channel-auth"}
		cfg.Profiles["audit"] = p
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(config.ResolvePath())
	if err != nil {
		t.Fatal(err)
	}
	mgmt := NewMgmtRouter()
	for _, endpoint := range []string{"test", "fetch-models"} {
		w := httptest.NewRecorder()
		mgmt.ServeHTTP(w, httptest.NewRequest("POST", "/api/profiles/audit/"+endpoint, nil))
		var result map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || (endpoint == "test" && result["success"] != true) {
			t.Errorf("%s rejected configured headers: HTTP %d %s, err=%v", endpoint, w.Code, w.Body.String(), err)
		}
	}
	after, err := os.ReadFile(config.ResolvePath())
	if err != nil || string(before) != string(after) {
		t.Fatal("read-only commands changed config")
	}
	db, err := store.GetDB()
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM requests`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("read-only commands wrote request facts: count=%d err=%v", count, err)
	}
	w := httptest.NewRecorder()
	mgmt.ServeHTTP(w, httptest.NewRequest("POST", "/api/profiles/audit/fetch-models?channel=main", nil))
	if w.Code != 200 {
		t.Errorf("channel fetch missed inherited policy: HTTP %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	NewProxyRouter().ServeHTTP(w, auditRequest("/v1/chat/completions", false))
	if w.Code != 200 {
		t.Errorf("proxy headers drifted: HTTP %d %s", w.Code, w.Body.String())
	}
}
