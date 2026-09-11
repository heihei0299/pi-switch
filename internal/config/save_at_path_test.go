package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Spec 08: every entry point (server / TUI / CLI) saves through SaveAtPath, so
// this is the single place that must apply the save-time migration.

func TestSaveAtPath_AppliesMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.json") // parent does not exist yet

	// A legacy-shaped config: no conversationSource, and fields that must not
	// survive a save. (LoadConfigAtPath already normalizes version, so the
	// migration is observed through the saved product rather than the input.)
	cfg, _, err := LoadConfigAtPath(writeFile(t, filepath.Join(dir, "src.json"),
		`{"profiles":{},"settings":{"injectOpenCodeAttribution":true}}`))
	if err != nil {
		t.Fatalf("load source: %v", err)
	}
	cfg.Settings.ConversationSource = "" // zero value still needs the save-time migration
	if cfg.Version != 2 {
		t.Fatalf("precondition: loaded version = %d, want 2", cfg.Version)
	}

	if err := SaveAtPath(cfg, path); err != nil {
		t.Fatalf("SaveAtPath: %v", err)
	}

	raw := readFile(t, path)
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("saved file is not valid JSON: %v", err)
	}
	if v, _ := m["version"].(float64); v != 2 {
		t.Fatalf("saved version = %v, want 2 (migration must run)", m["version"])
	}
	settings, _ := m["settings"].(map[string]interface{})
	if settings == nil {
		t.Fatalf("saved config has no settings object: %s", raw)
	}
	if settings["conversationSource"] != "sessionScan" {
		t.Fatalf("conversationSource = %v, want sessionScan (migration default)",
			settings["conversationSource"])
	}
	if _, ok := settings["injectOpenCodeAttribution"]; ok {
		t.Fatalf("legacy field survived the save: %s", raw)
	}
	if got := raw[len(raw)-1]; got != '\n' {
		t.Fatalf("saved file must end with a newline, got %q", got)
	}

	// The product must be loadable again as a valid config shape.
	reloaded, _, err := LoadConfigAtPath(path)
	if err != nil {
		t.Fatalf("reload saved config: %v", err)
	}
	if reloaded.Version != 2 {
		t.Fatalf("reloaded version = %d, want 2", reloaded.Version)
	}
	if reloaded.Settings.ConversationSource != "sessionScan" {
		t.Fatalf("reloaded source = %q, want sessionScan", reloaded.Settings.ConversationSource)
	}
}

func TestSaveAtPath_NormalizesInvalidConversationSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// A file with an invalid conversationSource no longer loads (strict reading),
	// so the save-time net is exercised with an in-memory value: the migration
	// must still normalize anything a caller hands to SaveAtPath.
	cfg := DefaultConfig()
	cfg.Settings.ConversationSource = "bogus"
	if err := SaveAtPath(cfg, path); err != nil {
		t.Fatalf("SaveAtPath: %v", err)
	}
	reloaded, _, err := LoadConfigAtPath(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Settings.ConversationSource != "sessionScan" {
		t.Fatalf("source = %q, want sessionScan", reloaded.Settings.ConversationSource)
	}
}

func TestSaveAtPath_AtomicWriteLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := SaveAtPath(DefaultConfig(), path); err != nil {
		t.Fatalf("SaveAtPath: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "config.json" {
			t.Fatalf("unexpected leftover file after atomic write: %s", e.Name())
		}
	}
}

func TestSaveAtPath_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("not json at all"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SaveAtPath(DefaultConfig(), path); err != nil {
		t.Fatalf("SaveAtPath over existing file: %v", err)
	}
	if _, _, err := LoadConfigAtPath(path); err != nil {
		t.Fatalf("overwritten file must be valid config: %v", err)
	}
}

func TestResolvePath_PrefersEnvVar(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom.json")
	t.Setenv("PI_SWITCH_CONFIG", want)
	if got := ResolvePath(); got != want {
		t.Fatalf("ResolvePath() = %q, want %q", got, want)
	}
}

func TestResolvePath_DefaultsUnderHome(t *testing.T) {
	t.Setenv("PI_SWITCH_CONFIG", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	got := ResolvePath()
	want := filepath.Join(home, ".pi-switch", "config.json")
	if got != want {
		t.Fatalf("ResolvePath() = %q, want %q", got, want)
	}
}

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ARCH-02: the config must never be world-readable. SaveAtPath writes through a
// 0600 temp file and the rename carries that mode onto the final file, even when
// the previous file was 0644.
func TestSaveAtPath_FileModeIs0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports mode bits differently")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SaveAtPath(DefaultConfig(), path); err != nil {
		t.Fatalf("SaveAtPath: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("config mode = %o, want 600", mode)
	}
}
