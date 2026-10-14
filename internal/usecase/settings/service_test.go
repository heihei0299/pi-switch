package settings

import (
	"context"
	"reflect"
	"testing"

	domainconfig "github.com/heihei0299/pi-switch/internal/domain/config"
	"github.com/heihei0299/pi-switch/internal/domain/revision"
)

type memoryConfigStore struct {
	cfg     domainconfig.PiSwitchConfig
	backups int
}

func (s *memoryConfigStore) Load() (domainconfig.PiSwitchConfig, string, error) {
	return s.cfg, "memory", nil
}

func (s *memoryConfigStore) UpdateConfig(_ context.Context, backup bool, mutate func(*domainconfig.PiSwitchConfig) error) error {
	cfg := s.cfg
	if err := mutate(&cfg); err != nil {
		return err
	}
	if backup {
		s.backups++
	}
	s.cfg = cfg
	return nil
}

func validSettings() domainconfig.Settings {
	settings := domainconfig.Settings{}
	settings.Proxy.Port = 43112
	settings.Web.Port = 43110
	return settings
}

func TestPutLegacyUpdatesLatestSettingsWithoutRevisionOrBackup(t *testing.T) {
	current := "supplier"
	old := validSettings()
	store := &memoryConfigStore{cfg: domainconfig.PiSwitchConfig{
		Current: &current, Profiles: map[string]domainconfig.ProviderProfile{"supplier": {API: "openai-chat"}}, Settings: old,
	}}
	service := NewService(store)
	updated := validSettings()
	updated.Proxy.Host = "127.0.0.2"
	if err := service.PutLegacy(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.cfg.Settings, updated) || store.cfg.Current == nil || *store.cfg.Current != current || len(store.cfg.Profiles) != 1 || store.backups != 0 {
		t.Fatalf("legacy settings update changed unrelated config or backup behavior: %+v, backups=%d", store.cfg, store.backups)
	}
}

func TestSaveChecksRevisionAndBacksUpOnlySuccessfulWrite(t *testing.T) {
	current := validSettings()
	store := &memoryConfigStore{cfg: domainconfig.PiSwitchConfig{Settings: current}}
	service := NewService(store)
	updated := validSettings()
	updated.Proxy.Host = "127.0.0.2"
	if err := service.Save(context.Background(), updated, revision.Of(current)); err != nil {
		t.Fatal(err)
	}
	if store.backups != 1 || !reflect.DeepEqual(store.cfg.Settings, updated) {
		t.Fatalf("save = %+v, backups=%d", store.cfg.Settings, store.backups)
	}
	if err := service.Save(context.Background(), current, revision.Of(current)); err != ErrRevisionConflict {
		t.Fatalf("stale settings save error = %v, want conflict", err)
	}
	if store.backups != 1 || !reflect.DeepEqual(store.cfg.Settings, updated) {
		t.Fatalf("conflict changed settings or created backup: %+v, backups=%d", store.cfg.Settings, store.backups)
	}
}
