package application

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupMetadataNewestFirstAndStableWhenTimesEqual(t *testing.T) {
	path, _ := managementFixture(t)
	if err := os.Mkdir(path+".backups", 0700); err != nil {
		t.Fatal(err)
	}
	old, recent := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC), time.Date(2026, 10, 10, 11, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		modified time.Time
	}{{"config-a.json", old}, {"config-b.json", recent}, {"config-c.json", recent}} {
		name := filepath.Join(path+".backups", tc.name)
		if err := os.WriteFile(name, []byte(`{"profiles":{}}`), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(name, tc.modified, tc.modified); err != nil {
			t.Fatal(err)
		}
	}
	items, err := ListConfigBackups(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].Name != "config-b.json" || items[1].Name != "config-c.json" || items[2].Name != "config-a.json" || !items[0].ModifiedAt.Equal(recent) {
		t.Fatalf("backup order/time = %+v", items)
	}
}

func TestManagementKnownValidationErrorsAreSafeAndActionable(t *testing.T) {
	path, cfg := managementFixture(t)
	assertSafe := func(err error, hint string) {
		t.Helper()
		var safe *ManagementValidationError
		if !errors.As(err, &safe) || !strings.Contains(safe.Message, hint) || strings.Contains(safe.Message, "fake-secret") {
			t.Fatalf("missing safe hint %q: %v", hint, err)
		}
	}
	settings := cfg.Settings
	settings.Proxy.Port = 0
	assertSafe(SaveSettings(path, settings, Revision(cfg.Settings)), "65535")
	if _, err := SaveSettingsDraft(path, `{"conversationSource":"fake-secret"}`, Revision(cfg.Settings)); err != nil {
		assertSafe(err, "settings JSON")
	} else {
		t.Fatal("invalid settings accepted")
	}
	assertSafe(RestoreConfigBackup(path, "../fake-secret", mustConfigFileRevision(t, path)), "backup name")
	if err := os.Mkdir(path+".backups", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path+".backups", 0755); err != nil {
		t.Fatal(err)
	}
	_, err := CreateConfigBackup(path)
	assertSafe(err, "private")
}
