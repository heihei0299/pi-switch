package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanSupportsLegacyAndCurrentPiSessionFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", dir)

	fixtures := map[string]string{
		"legacy.jsonl": "" +
			`{"type":"session","id":"legacy","timestamp":"2025-01-01T10:00:00Z","cwd":"/tmp/legacy-project"}` + "\n" +
			`{"type":"message","timestamp":"2025-01-01T10:01:00Z","message":{"role":"user","content":"Legacy prompt"}}` + "\n" +
			`{"type":"message","timestamp":"2025-01-01T10:02:00Z","message":{"role":"assistant","model":"legacy-model","usage":{"prompt_tokens":23}}}` + "\n",
		"current.jsonl": "" +
			`{"type":"session","version":3,"id":"current","timestamp":"2026-01-01T10:00:00Z","cwd":"/tmp/current-project"}` + "\n" +
			`{"type":"message","id":"entry1","parentId":null,"timestamp":"2026-01-01T10:01:00Z","message":{"role":"user","content":[{"type":"text","text":"Current prompt"}]}}` + "\n" +
			`{"type":"session_info","id":"entry2","parentId":"entry1","timestamp":"2026-01-01T10:01:30Z","name":"Current session"}` + "\n" +
			`{"type":"message","id":"entry3","parentId":"entry2","timestamp":"2026-01-01T10:02:00Z","message":{"role":"assistant","content":[{"type":"text","text":"Done"}],"model":"current-model","usage":{"input":41}}}` + "\n",
	}
	for name, contents := range fixtures {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}

	got := Scan()
	for id, want := range map[string]struct {
		title, lastActive, model string
		promptTokens             uint64
	}{
		"legacy":  {"Legacy prompt", "2025-01-01T10:02:00Z", "legacy-model", 23},
		"current": {"Current session", "2026-01-01T10:02:00Z", "current-model", 41},
	} {
		session, ok := got[id]
		if !ok {
			t.Fatalf("Scan() missing %q session; got %#v", id, got)
		}
		if session.Title != want.title || session.LastActiveAt == nil || *session.LastActiveAt != want.lastActive || session.Model == nil || *session.Model != want.model || session.PromptTokensHint == nil || *session.PromptTokensHint != want.promptTokens {
			t.Errorf("Scan()[%q] = %+v, want %+v", id, session, want)
		}
	}
}
