package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagementOverviewSourceAndCredentialsAreSafe(t *testing.T) {
	path, _ := managementFixture(t)
	snapshot, err := LoadManagementOverview(path)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Source != path || len(snapshot.Profiles) != 1 || snapshot.Profiles[0].Endpoint != "https://example.test" {
		t.Fatal("overview lost source or safe endpoint")
	}
	text := fmt.Sprintf("%+v", snapshot)
	for _, secret := range []string{"fake-top-key", "fake-channel-key", "fake-header-key", "fake-password", "fake-query-key"} {
		if strings.Contains(text, secret) {
			t.Fatal("overview leaked credentials")
		}
	}
}

func TestManagementOverviewMissingAndInvalidConfigAreNotFakeSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	snapshot, err := LoadManagementOverview(path)
	if err != nil || !strings.Contains(snapshot.Source, "no file") {
		t.Fatal("missing config was not labelled as defaults")
	}
	if err := os.WriteFile(path, []byte(`{"settings":{"conversationSource":"fake-secret-value"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManagementOverview(path); err == nil || strings.Contains(err.Error(), "fake-secret-value") {
		t.Fatal("invalid config returned success or exposed stored value")
	}
}
