package application

import (
	"encoding/json"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/profile"
)

func DuplicateProvider(path, source, name, expected string) error {
	return updateWithBackup(path, func(cfg *config.PiSwitchConfig) error {
		prof, ok := cfg.Profiles[source]
		if !ok || Revision(prof) != expected {
			return ErrManagementConflict
		}
		return providerValidationError(prof, profile.DuplicateInConfig(cfg, source, name))
	})
}

func ProviderPresetDraft(id string) (ProviderDraft, error) {
	for _, preset := range ProviderPresets() {
		if preset["id"] != id {
			continue
		}
		api, _ := preset["api"].(string)
		baseURL, _ := preset["baseUrl"].(string)
		channel := "primary"
		prof := config.ProviderProfile{API: api, ResponsesMode: "auto", Preset: &id, Upstreams: []config.Upstream{{Name: &channel, API: api, BaseURL: baseURL, ExposedModels: []string{}}}}
		ids, _ := preset["models"].([]string)
		for _, model := range ids {
			prof.Upstreams[0].Models = append(prof.Upstreams[0].Models, config.ModelEntry{ID: model})
		}
		if err := profile.ValidateProfile(prof); err != nil {
			return ProviderDraft{}, err
		}
		data, err := json.MarshalIndent(prof, "", "  ")
		return ProviderDraft{JSON: string(data)}, err
	}
	return ProviderDraft{}, &ManagementValidationError{Message: "Unknown or unsupported preset."}
}

type ProviderProbe struct {
	OK           bool
	Milliseconds int64
}

func TestProviderDraft(path, original, raw, expected string, keys map[string]string) (ProviderProbe, error) {
	prof, err := ProviderFromDraft(path, original, raw, expected, keys)
	if err != nil {
		return ProviderProbe{}, err
	}
	ok, _, ms := profile.TestProfileUpstream(prof)
	return ProviderProbe{OK: ok, Milliseconds: ms}, nil
}

type FetchedProviderDraft struct {
	JSON   string
	Models int
	Counts profile.EnrichCounts
}

// Fetching changes only the masked editor draft; Save remains explicit.
func FetchProviderDraftModels(path, original, raw, expected string, keys map[string]string, channel string) (FetchedProviderDraft, error) {
	prof, err := ProviderFromDraft(path, original, raw, expected, keys)
	if err != nil {
		return FetchedProviderDraft{}, err
	}
	ids, entries, counts, err := profile.FetchChannelModelEntries(prof, channel)
	if err != nil {
		return FetchedProviderDraft{}, &ManagementValidationError{Message: "Model fetch failed; check the channel and upstream connection. No config was written; response body and credentials are hidden."}
	}
	var safe config.ProviderProfile
	if err := json.Unmarshal([]byte(raw), &safe); err != nil {
		return FetchedProviderDraft{}, err
	}
	if err := profile.MergeChannelModels(&safe, channel, entries); err != nil {
		return FetchedProviderDraft{}, err
	}
	if err := profile.MergeChannelModels(&prof, channel, entries); err != nil {
		return FetchedProviderDraft{}, err
	}
	if err := profile.ValidateProfile(prof); err != nil {
		return FetchedProviderDraft{}, providerValidationError(prof, err)
	}
	// An upstream or catalog warning is untrusted text, not a status widget.
	if counts.Warning != "" {
		counts.Warning = "Some catalog metadata is unavailable; existing entries were preserved."
	}
	data, err := json.MarshalIndent(safe, "", "  ")
	return FetchedProviderDraft{JSON: string(data), Models: len(ids), Counts: counts}, err
}
