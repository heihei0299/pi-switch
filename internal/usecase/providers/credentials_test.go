package providers_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/domain/provider"
	"github.com/heihei0299/pi-switch/internal/usecase/providers"
)

func TestCredentialDraftMaskNeverSerializesSecretsAndRestoresExactValues(t *testing.T) {
	name := "primary"
	original := provider.Profile{
		BaseURL: "https://user:fake-password@example.test",
		APIKey:  "fake-profile-key",
		Headers: map[string]string{"Authorization": "Bearer fake-profile-header"},
		Upstreams: []provider.Upstream{{
			Name:    &name,
			BaseURL: "https://channel-user:fake-channel-password@example.test",
			APIKey:  "fake-channel-key",
			Headers: map[string]string{"X-Secret": "fake-channel-header"},
		}},
	}

	masked, retained, err := providers.MaskCredentials(original)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(masked)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fake-password", "fake-profile-key", "fake-profile-header", "fake-channel-password", "fake-channel-key", "fake-channel-header"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("masked draft leaked %q: %s", secret, encoded)
		}
	}
	if err := providers.RestoreCredentials(&masked, retained); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(masked, original) {
		t.Fatalf("restored profile = %#v, want %#v", masked, original)
	}

	masked.APIKey = providers.KeptCredential + "missing"
	if err := providers.RestoreCredentials(&masked, retained); !errors.Is(err, providers.ErrRetainedCredential) {
		t.Fatalf("unknown retained credential error = %v", err)
	}
}
