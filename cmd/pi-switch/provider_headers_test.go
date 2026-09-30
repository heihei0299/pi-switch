package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func TestProviderCLIProbeAndFetchUseInheritedHeaders(t *testing.T) {
	isolateCLI(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Tenant") != "channel" || r.Header.Get("X-Account") != "account" || r.Header.Get("Authorization") != "channel-auth" {
			w.WriteHeader(403)
			fmt.Fprint(w, "missing configured headers")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"header-model"}]}`)
	}))
	defer srv.Close()
	createOneChannelProfile(t, "headers")
	if err := config.UpdateAtPath(config.ResolvePath(), func(cfg *config.PiSwitchConfig) error {
		p := cfg.Profiles["headers"]
		p.BaseURL, p.APIKey = srv.URL, "automatic-key"
		p.Headers = map[string]string{"X-Tenant": "profile", "X-Account": "account", "Authorization": "profile-auth"}
		p.Upstreams[0].BaseURL = ""
		p.Upstreams[0].Headers = map[string]string{"x-tenant": "channel", "authorization": "channel-auth"}
		cfg.Profiles["headers"] = p
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(config.ResolvePath())
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"test", "fetch-models"} {
		code, out, errOut := runCLIStreams(t, func() int { return handleProvider([]string{command, "headers"}) })
		if code != 0 || (command == "fetch-models" && !strings.Contains(out, "header-model")) {
			t.Errorf("provider %s exit=%d out=%q err=%q", command, code, out, errOut)
		}
	}
	after, err := os.ReadFile(config.ResolvePath())
	if err != nil || string(before) != string(after) {
		t.Fatal("read-only CLI commands changed config")
	}
	code, out, errOut := runCLIStreams(t, func() int { return handleProvider([]string{"fetch-models", "headers", "--channel", "main"}) })
	if code != 0 || !strings.Contains(out, "header-model") {
		t.Fatalf("channel fetch missed inherited headers: exit=%d out=%q err=%q", code, out, errOut)
	}
	if shown := showProfile(t, "headers"); !strings.Contains(shown, `"header-model"`) {
		t.Fatalf("channel fetch did not persist new model: %s", shown)
	}
}
