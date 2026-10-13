package application

import (
	"encoding/json"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/profile"
	providerusecase "github.com/heihei0299/pi-switch/internal/usecase/providers"
)

const KeptCredential = providerusecase.KeptCredential

type ProviderDraft struct {
	JSON     string
	Revision string
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
	safe, _, err := providerusecase.MaskCredentials(prof)
	if err != nil {
		return ProviderDraft{}, err
	}
	if len(safe.Upstreams) == 0 || len(safe.Upstreams) == 1 && safe.ChannelName(0) == "" {
		profile.EnsureMutationChannel(&safe, "main")
	}
	data, err := json.MarshalIndent(safe, "", "  ")
	return ProviderDraft{JSON: string(data), Revision: Revision(prof)}, err
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
		_, retained, err = providerusecase.MaskCredentials(old)
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
	for i := range prof.Upstreams {
		u := &prof.Upstreams[i]
		if replacement := keys[prof.ChannelName(i)]; prof.ChannelName(i) != "" && replacement != "" {
			u.APIKey = replacement
		}
	}
	if err := providerusecase.RestoreCredentials(&prof, retained); err != nil {
		return prof, &ManagementValidationError{Message: err.Error()}
	}
	return prof, providerValidationError(prof, profile.ValidateProfile(prof))
}
