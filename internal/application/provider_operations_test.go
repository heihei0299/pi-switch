package application

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/profile"
	"github.com/heihei0299/pi-switch/internal/protocol"
)

func TestDuplicateProviderUsesSharedGateAndBacksUp(t *testing.T) {
	path, cfg := managementFixture(t)
	before, _ := os.ReadFile(path)
	source := cfg.Profiles["supplier"]
	if err := DuplicateProvider(path, "supplier", " supplier-copy ", Revision(source)); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if Revision(loaded.Profiles["supplier-copy"]) != Revision(source) || loaded.Current == nil || *loaded.Current != "supplier" {
		t.Fatal("copy lost data or switched current")
	}
	backups, _ := ListConfigBackups(path)
	if len(backups) != 1 {
		t.Fatalf("copy backups = %v", backups)
	}
	data, _ := os.ReadFile(filepath.Join(path+".backups", backups[0].Name))
	if string(data) != string(before) {
		t.Fatal("copy backup was not pre-write data")
	}
	before, _ = os.ReadFile(path)
	for _, tc := range []struct{ name, revision string }{{"supplier-copy", Revision(source)}, {"", Revision(source)}, {"other", "stale"}} {
		if err := DuplicateProvider(path, "supplier", tc.name, tc.revision); err == nil {
			t.Fatalf("invalid copy accepted: %q", tc.name)
		}
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("failed copy changed config")
	}
	backups, _ = ListConfigBackups(path)
	if len(backups) != 1 {
		t.Fatal("failed copy made backup")
	}
}

func TestDuplicateProviderPreservesAdvisoryLegacyShape(t *testing.T) {
	path, cfg := managementFixture(t)
	prof := cfg.Profiles["supplier"]
	prof.Upstreams[0].Name = nil
	cfg.Profiles["supplier"] = prof
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	if err := profile.ValidateProfile(prof); err == nil {
		t.Fatal("fixture needs advisory shape issue")
	}
	if err := DuplicateProvider(path, "supplier", "copy", Revision(prof)); err != nil {
		t.Fatalf("Native copy applied CRUD shape gate instead of shared copy gate: %v", err)
	}
}

func TestProviderPresetDraftsAreProxySupportedAndUnexposed(t *testing.T) {
	for _, preset := range ProviderPresets() {
		id := preset["id"].(string)
		draft, err := ProviderPresetDraft(id)
		if err != nil {
			t.Fatalf("preset %s: %v", id, err)
		}
		var prof config.ProviderProfile
		if err := json.Unmarshal([]byte(draft.JSON), &prof); err != nil {
			t.Fatal(err)
		}
		if !protocol.CanProxy(prof.API) || prof.APIKey != "" || len(prof.Upstreams) != 1 || prof.Upstreams[0].APIKey != "" || prof.Upstreams[0].ExposedModels == nil || len(prof.Upstreams[0].ExposedModels) != 0 {
			t.Fatalf("unsafe preset %s", id)
		}
		if err := profile.ValidateProfile(prof); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ProviderPresetDraft("unsupported"); err == nil {
		t.Fatal("unsupported preset accepted")
	}
}

func TestProviderProbeAndFetchUseSelectedCredentialsAndStayDraftOnly(t *testing.T) {
	var gotKey, gotShared, gotOverride string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey, gotShared, gotOverride = r.Header.Get("Authorization"), r.Header.Get("X-Shared"), r.Header.Get("X-Override")
		_, _ = w.Write([]byte(`{"data":[{"id":"one"},{"id":"fresh"},{"id":"unmatched"},{"id":"fresh"}]}`))
	}))
	defer server.Close()
	path, cfg := managementFixture(t)
	prof := cfg.Profiles["supplier"]
	prof.Headers = map[string]string{"X-Shared": "fake-shared", "X-Override": "profile"}
	prof.Upstreams[0].BaseURL, prof.Upstreams[0].Headers = server.URL, map[string]string{"X-Override": "primary"}
	prof.Upstreams[0].Models[0].ContextWindow = 4096
	name := "secondary"
	prof.Upstreams = append(prof.Upstreams, config.Upstream{Name: &name, API: prof.API, BaseURL: server.URL, APIKey: "fake-secondary-key", Headers: map[string]string{"X-Override": "secondary"}, Models: []config.ModelEntry{{ID: "one", ContextWindow: 8192}}, ExposedModels: []string{}})
	provider := "openai"
	prof.ModelsDevProvider = &provider
	cfg.Profiles["supplier"] = prof
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(t.TempDir(), "catalog.json")
	if err := os.WriteFile(catalog, []byte(`{"openai":{"models":{"one":{"limit":{"context":1000000}},"fresh":{"limit":{"context":32000,"output":4096},"cost":{"input":0.5,"output":1}}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_SWITCH_CATALOG", catalog)
	before, _ := os.ReadFile(path)
	draft, _ := LoadProviderDraft(path, "supplier")
	probe, err := TestProviderDraft(path, "supplier", draft.JSON, draft.Revision, nil)
	if err != nil || !probe.OK || gotKey != "Bearer fake-channel-key" || gotShared != "fake-shared" || gotOverride != "primary" {
		t.Fatalf("probe failed or did not use primary credentials: %v", err)
	}
	fetched, err := FetchProviderDraftModels(path, "supplier", draft.JSON, draft.Revision, nil, "secondary")
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "Bearer fake-secondary-key" || gotOverride != "secondary" || gotShared != "fake-shared" {
		t.Fatal("fetch did not use the selected channel/header inheritance")
	}
	var edited config.ProviderProfile
	if err := json.Unmarshal([]byte(fetched.JSON), &edited); err != nil {
		t.Fatal(err)
	}
	if len(edited.Upstreams[0].Models) != 1 || len(edited.Upstreams[1].Models) != 3 || edited.Upstreams[1].Models[0].ContextWindow != 8192 || len(edited.Upstreams[1].ExposedModels) != 0 || edited.Upstreams[1].ExposedModels == nil {
		t.Fatal("fetch overwrote metadata, changed another channel, exposed models, or duplicated IDs")
	}
	if edited.Upstreams[1].Models[1].ContextWindow != 32000 || edited.Upstreams[1].Models[1].Cost == nil || edited.Upstreams[1].Models[2].Cost != nil || fetched.Counts.Enriched != 3 || fetched.Counts.Skipped != 1 {
		t.Fatal("catalog metadata/unknown cost diverged from shared fetch")
	}
	for _, secret := range []string{"fake-channel-key", "fake-secondary-key", "fake-shared"} {
		if strings.Contains(fetched.JSON, secret) {
			t.Fatalf("fetched draft leaked %s", secret)
		}
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("probe/fetch wrote config")
	}
	backups, _ := ListConfigBackups(path)
	if len(backups) != 0 {
		t.Fatal("probe/fetch made backups before Save")
	}
	if _, err := FetchProviderDraftModels(path, "supplier", draft.JSON, "stale", nil, "secondary"); !errors.Is(err, ErrManagementConflict) {
		t.Fatalf("fetch accepted stale source: %v", err)
	}
	resolved, err := ProviderFromDraft(path, "supplier", fetched.JSON, draft.Revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Upstreams[1].APIKey != "fake-secondary-key" {
		t.Fatal("fetch lost kept credential marker")
	}
}

func TestProviderNetworkFailuresKeepFilesAndHideResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte("fake-upstream-private-body"))
	}))
	defer server.Close()
	path, cfg := managementFixture(t)
	prof := cfg.Profiles["supplier"]
	prof.Upstreams[0].BaseURL = server.URL
	cfg.Profiles["supplier"] = prof
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	draft, _ := LoadProviderDraft(path, "supplier")
	probe, err := TestProviderDraft(path, "supplier", draft.JSON, draft.Revision, nil)
	if err != nil || probe.OK {
		t.Fatalf("403 probe returned success: %v", err)
	}
	if _, err := FetchProviderDraftModels(path, "supplier", draft.JSON, draft.Revision, nil, "primary"); err == nil || strings.Contains(err.Error(), "fake-upstream-private-body") {
		t.Fatalf("model failure missing or leaked body: %v", err)
	}
	if _, err := FetchUpstreamUsage(server.URL, "fake-key", "fake-UA"); err == nil || !strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "fake-upstream-private-body") {
		t.Fatalf("usage failure missing or leaked body: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("network failure wrote config")
	}
}

func TestProviderDraftChannelRenameAndReorderKeepOriginalReferences(t *testing.T) {
	path, cfg := managementFixture(t)
	prof := cfg.Profiles["supplier"]
	second := "secondary"
	prof.Upstreams = append(prof.Upstreams, config.Upstream{Name: &second, API: prof.API, APIKey: "fake-second-key", BaseURL: "https://second.example.test/private-path", Headers: map[string]string{"Authorization": "fake-second-header"}})
	cfg.Profiles["supplier"] = prof
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	draft, err := LoadProviderDraft(path, "supplier")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(draft.JSON, "private-path") {
		t.Fatal("stored URL path was rendered")
	}
	var edited config.ProviderProfile
	if err := json.Unmarshal([]byte(draft.JSON), &edited); err != nil {
		t.Fatal(err)
	}
	renamed := "renamed"
	edited.Upstreams[1].Name = &renamed
	edited.Upstreams[0], edited.Upstreams[1] = edited.Upstreams[1], edited.Upstreams[0]
	raw, _ := json.Marshal(edited)
	resolved, err := ProviderFromDraft(path, "supplier", string(raw), draft.Revision, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Upstreams[0].APIKey != "fake-second-key" || resolved.Upstreams[0].Headers["Authorization"] != "fake-second-header" || resolved.Upstreams[0].BaseURL != prof.Upstreams[1].BaseURL || resolved.Upstreams[1].APIKey != "fake-channel-key" {
		t.Fatal("channel rename/reorder cross-wired credentials")
	}
	edited.Upstreams[0].APIKey = KeptCredential + "upstreams[99].apiKey"
	raw, _ = json.Marshal(edited)
	if _, err := ProviderFromDraft(path, "supplier", string(raw), draft.Revision, nil); err == nil {
		t.Fatal("forged credential reference accepted")
	}
}

func TestProviderDraftMaterializesLegacyChannelOnlyInEditor(t *testing.T) {
	for _, unnamed := range []bool{false, true} {
		path, cfg := managementFixture(t)
		prof := config.ProviderProfile{API: "openai-completions", BaseURL: "https://legacy.example.test/v1?key=fake-url-key", APIKey: "fake-legacy-key", Headers: map[string]string{"X-Legacy": "fake-legacy-header"}}
		if unnamed {
			prof.Upstreams = []config.Upstream{{APIKey: "fake-unnamed-key"}}
		}
		cfg.Profiles["supplier"] = prof
		if err := config.SaveAtPath(cfg, path); err != nil {
			t.Fatal(err)
		}
		before, _ := os.ReadFile(path)
		draft, err := LoadProviderDraft(path, "supplier")
		if err != nil {
			t.Fatal(err)
		}
		resolved, err := ProviderFromDraft(path, "supplier", draft.JSON, draft.Revision, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(resolved.Upstreams) != 1 || resolved.ChannelName(0) != "main" || resolved.Upstreams[0].BaseURL != prof.BaseURL || resolved.PrimaryHeaders()["X-Legacy"] != "fake-legacy-header" {
			t.Fatal("legacy editor did not preserve fallback semantics")
		}
		wantKey := "fake-legacy-key"
		if unnamed {
			wantKey = "fake-unnamed-key"
		}
		if resolved.PrimaryAPIKey() != wantKey {
			t.Fatal("legacy editor changed effective credential")
		}
		after, _ := os.ReadFile(path)
		if string(before) != string(after) {
			t.Fatal("opening legacy editor migrated persistent data")
		}
	}
}
