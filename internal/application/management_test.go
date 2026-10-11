package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func managementFixture(t *testing.T) (string, config.PiSwitchConfig) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	name, channel := "supplier", "primary"
	cfg.Current = &name
	cfg.Profiles = map[string]config.ProviderProfile{name: {
		API: "openai-completions", APIKey: "fake-top-key", Headers: map[string]string{"Authorization": "fake-header-key"},
		Upstreams: []config.Upstream{{Name: &channel, API: "openai-completions", BaseURL: "https://fake-user:fake-password@example.test/v1?key=fake-query-key", APIKey: "fake-channel-key", Models: []config.ModelEntry{{ID: "one"}}, ExposedModels: []string{}}},
	}}
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	return path, cfg
}

func TestProviderDraftMasksAndPreservesCredentialsOnRename(t *testing.T) {
	path, cfg := managementFixture(t)
	draft, err := LoadProviderDraft(path, "supplier")
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fake-top-key", "fake-channel-key", "fake-header-key", "fake-password", "fake-query-key"} {
		if strings.Contains(draft.JSON, secret) {
			t.Fatalf("draft leaked %s", secret)
		}
	}
	prof, err := ProviderFromDraft(path, "supplier", draft.JSON, draft.Revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	if Revision(prof) != Revision(cfg.Profiles["supplier"]) {
		t.Fatal("masked draft lost provider fields or credentials")
	}
	if err := SaveProvider(path, "supplier", "renamed", prof, draft.Revision); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Current == nil || *loaded.Current != "renamed" {
		t.Fatal("rename left dangling current profile")
	}
	if Revision(loaded.Profiles["renamed"]) != Revision(prof) {
		t.Fatal("save lost explicit empty exposure or credentials")
	}
}

func TestProviderValidationConflictAndCancelHaveNoWrites(t *testing.T) {
	path, cfg := managementFixture(t)
	before, _ := os.ReadFile(path)
	draft, _ := LoadProviderDraft(path, "supplier")
	if _, err := ProviderFromDraft(path, "supplier", "{", draft.Revision, nil); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if _, err := ProviderFromDraft(path, "new", draft.JSON, draft.Revision, nil); err == nil {
		t.Fatal("stale provider accepted")
	}
	if err := SaveProvider(path, "supplier", "other", cfg.Profiles["supplier"], "stale"); !errors.Is(err, ErrManagementConflict) {
		t.Fatalf("conflict = %v", err)
	}
	if err := DeleteProvider(path, "supplier", "stale"); !errors.Is(err, ErrManagementConflict) {
		t.Fatalf("delete conflict = %v", err)
	}
	if err := SaveProvider(path, "", "supplier", cfg.Profiles["supplier"], ""); err == nil {
		t.Fatal("duplicate overwrote supplier")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("failed action wrote config")
	}
	backups, err := ListConfigBackups(path)
	if err != nil || len(backups) != 0 {
		t.Fatalf("failed actions wrote backups: %v %v", backups, err)
	}
}

func TestSettingsBackupRestoreAndPermissions(t *testing.T) {
	path, cfg := managementFixture(t)
	before, _ := os.ReadFile(path)
	settings := cfg.Settings
	settings.Proxy.Port++
	if err := SaveSettings(path, settings, Revision(cfg.Settings)); err != nil {
		t.Fatal(err)
	}
	backups, err := ListConfigBackups(path)
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v %v", backups, err)
	}
	backupPath := filepath.Join(path+".backups", backups[0].Name)
	data, _ := os.ReadFile(backupPath)
	if string(data) != string(before) {
		t.Fatal("backup not exact pre-write bytes")
	}
	info, _ := os.Stat(backupPath)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("backup permissions = %o", info.Mode().Perm())
	}
	revision, _ := ConfigFileRevision(path)
	if err := RestoreConfigBackup(path, backups[0].Name, "stale"); !errors.Is(err, ErrManagementConflict) {
		t.Fatalf("restore conflict = %v", err)
	}
	if err := RestoreConfigBackup(path, "../config.json", revision); err == nil {
		t.Fatal("backup traversal accepted")
	}
	if err := RestoreConfigBackup(path, backups[0].Name, revision); err != nil {
		t.Fatal(err)
	}
	restored, _, _ := config.LoadConfigAtPath(path)
	if Revision(restored) != Revision(cfg) {
		t.Fatal("restore changed data contract")
	}
	backups, _ = ListConfigBackups(path)
	if len(backups) != 2 {
		t.Fatal("restore did not back up replaced config")
	}
}

func TestBackupFailureAndInvalidSettingsDoNotWrite(t *testing.T) {
	path, cfg := managementFixture(t)
	before, _ := os.ReadFile(path)
	if err := os.WriteFile(path+".backups", []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	settings := cfg.Settings
	settings.Proxy.Port++
	if err := SaveSettings(path, settings, Revision(cfg.Settings)); err == nil {
		t.Fatal("backup failure ignored")
	}
	settings.Proxy.Port = 0
	if err := SaveSettings(path, settings, Revision(cfg.Settings)); err == nil {
		t.Fatal("invalid port accepted")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("failed save changed config")
	}
}

func TestRestoreRejectsInvalidOrSymlinkBackup(t *testing.T) {
	path, _ := managementFixture(t)
	name, err := CreateConfigBackup(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path+".backups", name), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RestoreConfigBackup(path, name, mustConfigFileRevision(t, path)); err == nil {
		t.Fatal("invalid backup accepted")
	}
	link := filepath.Join(path+".backups", "config-link.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err := RestoreConfigBackup(path, "config-link.json", mustConfigFileRevision(t, path)); err == nil {
		t.Fatal("symlink backup accepted")
	}
	backups, _ := ListConfigBackups(path)
	if len(backups) != 1 {
		t.Fatal("backup list included symlink")
	}
}

func mustConfigFileRevision(t *testing.T, path string) string {
	t.Helper()
	revision, err := ConfigFileRevision(path)
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func TestBackupRecoversUnreadableConfigAndPreservesBrokenBytes(t *testing.T) {
	path, cfg := managementFixture(t)
	name, err := CreateConfigBackup(path)
	if err != nil {
		t.Fatal(err)
	}
	broken := []byte(`{"profiles": invalid`)
	if err := os.WriteFile(path, broken, 0600); err != nil {
		t.Fatal(err)
	}
	if err := RestoreConfigBackup(path, name, mustConfigFileRevision(t, path)); err != nil {
		t.Fatal(err)
	}
	restored, _, err := config.LoadConfigAtPath(path)
	if err != nil || Revision(restored) != Revision(cfg) {
		t.Fatalf("failed recovery: %v", err)
	}
	items, _ := ListConfigBackups(path)
	found := false
	for _, item := range items {
		data, _ := os.ReadFile(filepath.Join(path+".backups", item.Name))
		found = found || string(data) == string(broken)
	}
	if !found {
		t.Fatal("recovery did not preserve broken source")
	}
}

func TestProviderDraftRestoresByChannelNameNotPosition(t *testing.T) {
	path, cfg := managementFixture(t)
	prof := cfg.Profiles["supplier"]
	secondary := "secondary"
	prof.Upstreams = append(prof.Upstreams, config.Upstream{Name: &secondary, API: "openai-completions", BaseURL: "https://secondary.example.test", APIKey: "fake-secondary-key"})
	cfg.Profiles["supplier"] = prof
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	draft, err := LoadProviderDraft(path, "supplier")
	if err != nil {
		t.Fatal(err)
	}
	var edited config.ProviderProfile
	if err := json.Unmarshal([]byte(draft.JSON), &edited); err != nil {
		t.Fatal(err)
	}
	edited.Upstreams[0], edited.Upstreams[1] = edited.Upstreams[1], edited.Upstreams[0]
	raw, _ := json.Marshal(edited)
	resolved, err := ProviderFromDraft(path, "supplier", string(raw), draft.Revision, map[string]string{"primary": "fake-new-primary-key"})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Upstreams[0].APIKey != "fake-secondary-key" || resolved.Upstreams[1].APIKey != "fake-new-primary-key" {
		t.Fatal("reordering cross-wired credentials")
	}
	if _, err := ProviderFromDraft(path, "supplier", string(raw), draft.Revision, map[string]string{"unknown": "fake-key"}); err == nil {
		t.Fatal("unknown credential target accepted")
	}
}

func TestValidationErrorRedactsSecretsAndKeepsFieldDiagnostic(t *testing.T) {
	prof := config.ProviderProfile{APIKey: "fake-key", Headers: map[string]string{"Authorization": "fake-header"}, BaseURL: "https://user:fake-password@example.test/?key=fake-query"}
	err := providerValidationError(prof, errors.New("api fake-key / fake-header / fake-password / fake-query is unsupported"))
	var safe *ManagementValidationError
	if !errors.As(err, &safe) || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("missing validation diagnostic: %v", err)
	}
	for _, secret := range []string{"fake-key", "fake-header", "fake-password", "fake-query"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("diagnostic leaked %s", secret)
		}
	}
}

func TestValidationErrorRedactsQuotedCredentials(t *testing.T) {
	secret := "fake-\"quoted\nkey"
	prof := config.ProviderProfile{APIKey: secret}
	err := providerValidationError(prof, fmt.Errorf("requestScopedErrors[0].action got %q", secret))
	if strings.Contains(err.Error(), "quoted") || !strings.Contains(err.Error(), "requestScopedErrors[0].action") {
		t.Fatalf("quoted credential leaked or field diagnostic lost: %v", err)
	}
}

func TestBackupRejectsNonPrivateDirectory(t *testing.T) {
	path, _ := managementFixture(t)
	if err := os.Mkdir(path+".backups", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path+".backups", 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateConfigBackup(path); err == nil {
		t.Fatal("public backup directory accepted")
	}
}

func TestRestoreRejectsUnsupportedCapabilityWithoutWrites(t *testing.T) {
	path, _ := managementFixture(t)
	name, err := CreateConfigBackup(path)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	invalid := []byte(`{"profiles":{"bad":{"api":"unsupported","baseUrl":"https://example.test"}}}`)
	if err := os.WriteFile(filepath.Join(path+".backups", name), invalid, 0600); err != nil {
		t.Fatal(err)
	}
	if err := RestoreConfigBackup(path, name, mustConfigFileRevision(t, path)); err == nil {
		t.Fatal("unsupported backup accepted")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("invalid restoration wrote config")
	}
	items, _ := ListConfigBackups(path)
	if len(items) != 1 {
		t.Fatal("invalid restoration created backup")
	}
}

func TestSettingsDraftKeepsNormalizedRevisionAndRejectsInvalidDraft(t *testing.T) {
	path, _ := managementFixture(t)
	draft, err := LoadSettingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	var settings config.Settings
	if err := json.Unmarshal([]byte(draft.JSON), &settings); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(settings)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "conversationSource")
	raw, _ = json.Marshal(fields)
	saved, err := SaveSettingsDraft(path, string(raw), draft.Revision)
	if err != nil {
		t.Fatal(err)
	}
	actual, _, _ := config.LoadConfigAtPath(path)
	if saved.Revision != Revision(actual.Settings) || !strings.Contains(saved.JSON, `"sessionScan"`) {
		t.Fatal("returned settings revision is not the persisted normalized revision")
	}
	before, _ := os.ReadFile(path)
	if _, err := SaveSettingsDraft(path, "{fake-secret", saved.Revision); err == nil || strings.Contains(err.Error(), "fake-secret") {
		t.Fatal("invalid settings draft accepted or leaked")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("invalid settings draft wrote config")
	}
}

func TestBackupDirectorySymlinkIsNeverConsumed(t *testing.T) {
	path, _ := managementFixture(t)
	other := t.TempDir()
	if err := os.Chmod(other, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "config-other.json"), []byte(`{"profiles":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, path+".backups"); err != nil {
		t.Fatal(err)
	}
	if _, err := ListConfigBackups(path); err == nil {
		t.Fatal("list followed backup directory symlink")
	}
	if _, err := CreateConfigBackup(path); err == nil {
		t.Fatal("create followed backup directory symlink")
	}
	if err := RestoreConfigBackup(path, "config-other.json", mustConfigFileRevision(t, path)); err == nil {
		t.Fatal("restore followed backup directory symlink")
	}
}
