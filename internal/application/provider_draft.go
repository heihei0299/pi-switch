package application

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/profile"
)

const KeptCredential = "__PI_SWITCH_KEEP_CREDENTIAL__"

type ProviderDraft struct {
	JSON     string
	Revision string
}

// References point into the revision-checked original, not the draft's current
// channel position/name, so reordering or renaming cannot cross-wire credentials.
func maskedProvider(prof config.ProviderProfile) (config.ProviderProfile, map[string]string, error) {
	data, err := json.Marshal(prof)
	if err != nil {
		return config.ProviderProfile{}, nil, err
	}
	var safe config.ProviderProfile
	if err := json.Unmarshal(data, &safe); err != nil {
		return safe, nil, err
	}
	retained := make(map[string]string)
	mask := func(value *string, field string) {
		if *value == "" {
			return
		}
		reference := KeptCredential + field
		retained[reference], *value = *value, reference
	}
	credentials := func(endpoint, key *string, headers map[string]string, scope string) {
		mask(endpoint, scope+".baseUrl")
		mask(key, scope+".apiKey")
		for name, value := range headers {
			mask(&value, scope+".headers."+name)
			headers[name] = value
		}
	}
	credentials(&safe.BaseURL, &safe.APIKey, safe.Headers, "profile")
	for i := range safe.Upstreams {
		u := &safe.Upstreams[i]
		credentials(&u.BaseURL, &u.APIKey, u.Headers, fmt.Sprintf("upstreams[%d]", i))
	}
	return safe, retained, nil
}

func LoadProviderDraft(path, name string) (ProviderDraft, error) {
	cfg, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		return ProviderDraft{}, err
	}
	prof, ok := cfg.Profiles[name]
	if !ok {
		return ProviderDraft{}, profile.ErrProfileNotFound
	}
	safe, _, err := maskedProvider(prof)
	if err != nil {
		return ProviderDraft{}, err
	}
	if len(safe.Upstreams) == 0 || len(safe.Upstreams) == 1 && safe.ChannelName(0) == "" {
		profile.EnsureMutationChannel(&safe, "main")
	}
	data, err := json.MarshalIndent(safe, "", "  ")
	return ProviderDraft{JSON: string(data), Revision: Revision(prof)}, err
}

func restoreCredentials(endpoint, key *string, headers map[string]string, retained map[string]string) error {
	restore := func(value *string) error {
		if !strings.HasPrefix(*value, KeptCredential) {
			return nil
		}
		original, ok := retained[*value]
		if !ok {
			return &ManagementValidationError{Message: "retained credential has no matching source; reload or enter a replacement"}
		}
		*value = original
		return nil
	}
	if err := restore(endpoint); err != nil {
		return err
	}
	if err := restore(key); err != nil {
		return err
	}
	for name, value := range headers {
		if err := restore(&value); err != nil {
			return err
		}
		headers[name] = value
	}
	return nil
}

// ProviderFromDraft is read-only; validation and cancellation never create backups.
func ProviderFromDraft(path, original, raw, expected string, keys map[string]string) (config.ProviderProfile, error) {
	var prof config.ProviderProfile
	if err := json.Unmarshal([]byte(raw), &prof); err != nil {
		return prof, &ManagementValidationError{Message: "invalid provider JSON; draft retained"}
	}
	retained := make(map[string]string)
	if original != "" {
		cfg, _, err := config.LoadConfigAtPath(path)
		if err != nil {
			return prof, err
		}
		old, ok := cfg.Profiles[original]
		if !ok || Revision(old) != expected {
			return prof, ErrManagementConflict
		}
		_, retained, err = maskedProvider(old)
		if err != nil {
			return prof, err
		}
	}
	for name, replacement := range keys {
		if name == "" || replacement == "" {
			continue
		}
		found := false
		for i := range prof.Upstreams {
			found = found || prof.ChannelName(i) == name
		}
		if !found {
			return prof, &ManagementValidationError{Message: "replacement key channel does not exist in the draft"}
		}
	}
	if replacement := keys[""]; replacement != "" {
		prof.APIKey = replacement
	}
	if err := restoreCredentials(&prof.BaseURL, &prof.APIKey, prof.Headers, retained); err != nil {
		return prof, err
	}
	for i := range prof.Upstreams {
		u := &prof.Upstreams[i]
		if replacement := keys[prof.ChannelName(i)]; prof.ChannelName(i) != "" && replacement != "" {
			u.APIKey = replacement
		}
		if err := restoreCredentials(&u.BaseURL, &u.APIKey, u.Headers, retained); err != nil {
			return prof, err
		}
	}
	return prof, providerValidationError(prof, profile.ValidateProfile(prof))
}
