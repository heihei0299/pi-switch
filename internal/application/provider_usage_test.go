package application

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func TestProviderUsageReturnsRealWindowsAndUnknownNotZero(t *testing.T) {
	var gotKey, gotUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey, gotUA = r.Header.Get("Authorization"), r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(`{"usage":{"rolling":{"percent":0,"status":"ok","resetsAt":"2026-10-12T00:00:00Z"},"weekly":{"percent":23.75,"status":"fake-key-body","resetsAt":"fake-secret-reset"},"monthly":{"status":"ok"}}}`))
	}))
	defer server.Close()
	path, cfg := managementFixture(t)
	prof := cfg.Profiles["supplier"]
	prof.Upstreams[0].BaseURL = server.URL
	ua := "fake-profile-UA"
	prof.UserAgent = &ua
	cfg.Profiles["supplier"] = prof
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	windows, err := LoadProviderUsage(path, "supplier", Revision(prof))
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "Bearer fake-channel-key" || gotUA != ua {
		t.Fatal("usage credential/UA inheritance diverged")
	}
	if len(windows) != 3 || windows[0].Percent == nil || *windows[0].Percent != 0 || windows[0].ResetsAt != "2026-10-12T00:00:00Z" || windows[1].Percent == nil || *windows[1].Percent != 23.75 || windows[1].Status != "unknown" || windows[1].ResetsAt != "" || windows[2].Percent != nil {
		t.Fatalf("usage values fabricated or unsafe strings displayed: %#v", windows)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("usage query wrote config")
	}
	if _, err := LoadProviderUsage(path, "supplier", "stale"); err == nil {
		t.Fatal("usage accepted stale provider")
	}
}

func TestProviderUsageNoModelsAndMalformedBodyAreNotSuccess(t *testing.T) {
	path, cfg := managementFixture(t)
	prof := cfg.Profiles["supplier"]
	prof.Upstreams[0].Models = nil
	cfg.Profiles["supplier"] = prof
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	if windows, err := LoadProviderUsage(path, "supplier", Revision(prof)); err == nil || len(windows) != 0 {
		t.Fatal("no models returned fake zero usage")
	}
	for _, body := range []string{`{ fake-sensitive-body`, `{"other":{"percent":90}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		_, err := FetchUpstreamUsage(server.URL, "fake-key", "fake-UA")
		server.Close()
		if err == nil || strings.Contains(err.Error(), "fake-sensitive-body") {
			t.Fatalf("malformed usage missing error or leaked body: %v", err)
		}
	}
}
