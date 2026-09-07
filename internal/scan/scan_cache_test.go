package scan

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanCachedReusesSessionSnapshotWithinWindow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", dir)

	firstPath := filepath.Join(dir, "first.jsonl")
	first := `{"type":"session","id":"first","cwd":"/tmp/project","timestamp":"2026-09-07T21:00:00Z"}
`
	if err := os.WriteFile(firstPath, []byte(first), 0644); err != nil {
		t.Fatal(err)
	}

	initial := ScanCached()
	if len(initial) != 1 {
		t.Fatalf("initial scan found %d sessions, want 1", len(initial))
	}
	if _, ok := initial["first"]; !ok {
		t.Fatalf("initial scan missing first session: %#v", initial)
	}

	secondPath := filepath.Join(dir, "second.jsonl")
	second := `{"type":"session","id":"second","cwd":"/tmp/project","timestamp":"2026-09-07T21:00:01Z"}
`
	if err := os.WriteFile(secondPath, []byte(second), 0644); err != nil {
		t.Fatal(err)
	}

	cached := ScanCached()
	if len(cached) != 1 {
		t.Fatalf("cached scan found %d sessions, want the cached 1-session snapshot", len(cached))
	}
	if _, ok := cached["first"]; !ok {
		t.Fatalf("cached scan lost first session: %#v", cached)
	}
}

func TestScanCachedRefreshesExpiredSnapshotInBackground(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", dir)

	firstPath := filepath.Join(dir, "first.jsonl")
	first := `{"type":"session","id":"first","cwd":"/tmp/project","timestamp":"2026-09-07T21:00:00Z"}
`
	if err := os.WriteFile(firstPath, []byte(first), 0644); err != nil {
		t.Fatal(err)
	}
	if got := ScanCached(); len(got) != 1 {
		t.Fatalf("initial scan found %d sessions, want 1", len(got))
	}

	secondPath := filepath.Join(dir, "second.jsonl")
	second := `{"type":"session","id":"second","cwd":"/tmp/project","timestamp":"2026-09-07T21:00:01Z"}
`
	if err := os.WriteFile(secondPath, []byte(second), 0644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(sessionScanCacheTTL + 50*time.Millisecond)
	start := time.Now()
	stale := ScanCached()
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("expired cache blocked for %s, want stale snapshot to return immediately", elapsed)
	}
	if len(stale) != 1 {
		t.Fatalf("stale scan found %d sessions, want the previous 1-session snapshot", len(stale))
	}

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if refreshed := ScanCached(); len(refreshed) == 2 {
			if _, ok := refreshed["second"]; ok {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("expired session snapshot was not refreshed in the background")
}
