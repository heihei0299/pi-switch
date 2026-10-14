package providers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/domain/provider"
)

type testStore struct {
	state   State
	calls   int
	backups []bool
}

func (s *testStore) Update(ctx context.Context, backup bool, mutate func(*State) error) error {
	s.calls++
	if err := ctx.Err(); err != nil {
		return err
	}
	next := cloneState(s.state)
	if err := mutate(&next); err != nil {
		return err
	}
	s.state = next
	s.backups = append(s.backups, backup)
	return nil
}

type testValidator struct {
	err   error
	calls int
}

func (v *testValidator) Validate(context.Context, provider.Profile) error {
	v.calls++
	return v.err
}
func (v *testValidator) ValidateDuplicate(context.Context, provider.Profile) error {
	v.calls++
	return v.err
}

func cloneState(in State) State {
	out := State{Profiles: make(map[string]StoredProfile, len(in.Profiles))}
	if in.Current != nil {
		current := *in.Current
		out.Current = &current
	}
	for name, profile := range in.Profiles {
		out.Profiles[name] = profile
	}
	return out
}

func TestPutLegacyKeepsHTTPRenameAndBackupSemantics(t *testing.T) {
	current := "source"
	store := &testStore{state: State{
		Current: &current,
		Profiles: map[string]StoredProfile{
			"source": {Profile: provider.Profile{API: "source"}, Revision: "source-revision"},
			"target": {Profile: provider.Profile{API: "old-target"}, Revision: "target-revision"},
		},
	}}
	service := New(store, &testValidator{})
	result, err := service.PutLegacy(context.Background(), PutLegacyCommand{
		Name: "target", RenameFrom: "source", Profile: provider.Profile{API: "replacement"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "target" {
		t.Fatalf("result = %+v, want legacy no-backup update", result)
	}
	if store.state.Current == nil || *store.state.Current != "source" {
		t.Fatalf("legacy rename changed current: %v", store.state.Current)
	}
	if _, ok := store.state.Profiles["source"]; ok {
		t.Fatal("legacy rename kept renameFrom profile")
	}
	if got := store.state.Profiles["target"].Profile.API; got != "replacement" {
		t.Fatalf("target API = %s", got)
	}
	if len(store.backups) != 1 || store.backups[0] {
		t.Fatalf("backup calls = %v, want one no-backup mutation", store.backups)
	}
}

func TestCreateLegacyTrimsChecksAndDoesNotBackup(t *testing.T) {
	store := &testStore{state: State{Profiles: map[string]StoredProfile{}}}
	validator := &testValidator{}
	service := New(store, validator)
	result, err := service.CreateLegacy(context.Background(), " new ", provider.Profile{API: "openai-completions"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "new" || store.state.Profiles["new"].Profile.API != "openai-completions" || len(store.backups) != 1 || store.backups[0] {
		t.Fatalf("result/state/backups = %+v/%+v/%v", result, store.state, store.backups)
	}
	if _, err := service.CreateLegacy(context.Background(), "new", provider.Profile{}); !errors.Is(err, ErrProfileExists) {
		t.Fatalf("duplicate create error = %v, want ErrProfileExists", err)
	}
	if len(store.state.Profiles) != 1 || len(store.backups) != 1 {
		t.Fatal("duplicate create changed state or created backup")
	}
}

func TestDuplicateLegacyKeepsErrorOrderAndNoBackupContract(t *testing.T) {
	store := &testStore{state: State{Profiles: map[string]StoredProfile{
		"source": {Profile: provider.Profile{API: "openai-completions"}},
		"target": {Profile: provider.Profile{API: "anthropic-messages"}},
	}}}
	validator := &testValidator{}
	service := New(store, validator)
	if _, err := service.DuplicateLegacy(context.Background(), "missing", "target"); !errors.Is(err, ErrProfileNotFound) || err.Error() != `profile "missing" not found` {
		t.Fatalf("missing source error = %v", err)
	}
	if _, err := service.DuplicateLegacy(context.Background(), "source", "target"); !errors.Is(err, ErrProfileExists) || err.Error() != `target "target" already exists` {
		t.Fatalf("existing target error = %v", err)
	}
	if validator.calls != 0 || len(store.backups) != 0 {
		t.Fatal("duplicate conflicts reached validation or wrote a backup")
	}
	result, err := service.DuplicateLegacy(context.Background(), "source", "copy")
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "copy" || store.state.Profiles["copy"].Profile.API != "openai-completions" || validator.calls != 1 || len(store.backups) != 1 || store.backups[0] {
		t.Fatalf("result/state/validation/backups = %+v/%+v/%d/%v", result, store.state, validator.calls, store.backups)
	}
}

func TestValidationRunsBeforeMutation(t *testing.T) {
	validationErr := errors.New("invalid profile")
	store := &testStore{state: State{Profiles: map[string]StoredProfile{}}}
	service := New(store, &testValidator{err: validationErr})
	_, err := service.PutLegacy(context.Background(), PutLegacyCommand{Name: "new", Profile: provider.Profile{API: "bad"}})
	var validation *ValidationError
	if !errors.As(err, &validation) || !errors.Is(err, validationErr) {
		t.Fatalf("error = %v, want wrapped validation error", err)
	}
	if store.calls != 0 {
		t.Fatalf("store called %d times before validation succeeded", store.calls)
	}
}

func TestDeleteLegacyClearsCurrentAndMissingIsReadOnly(t *testing.T) {
	current := "supplier"
	store := &testStore{state: State{
		Current:  &current,
		Profiles: map[string]StoredProfile{"supplier": {Profile: provider.Profile{API: "openai-completions"}}},
	}}
	service := New(store, &testValidator{})
	result, err := service.DeleteLegacy(context.Background(), DeleteLegacyCommand{Name: "supplier"})
	if err != nil {
		t.Fatal(err)
	}
	if store.state.Current != nil {
		t.Fatalf("result/current = %+v/%v, want no backup and no current", result, store.state.Current)
	}
	if len(store.state.Profiles) != 0 || len(store.backups) != 1 || store.backups[0] {
		t.Fatalf("profiles/backups = %+v/%v", store.state.Profiles, store.backups)
	}
	before := cloneState(store.state)
	calls := store.calls
	_, err = service.DeleteLegacy(context.Background(), DeleteLegacyCommand{Name: "missing"})
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("missing delete error = %v", err)
	}
	if store.calls != calls+1 || len(store.backups) != 1 || store.state.Current != before.Current || len(store.state.Profiles) != len(before.Profiles) {
		t.Fatal("missing delete changed state or wrote a backup")
	}
}

func TestSelectLegacyUsesNoBackupAndRejectsMissingProfile(t *testing.T) {
	store := &testStore{state: State{Profiles: map[string]StoredProfile{
		"one": {Profile: provider.Profile{API: "openai-completions"}},
	}}}
	service := New(store, &testValidator{})
	if _, err := service.SelectLegacy(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	if store.state.Current == nil || *store.state.Current != "one" || len(store.backups) != 1 || store.backups[0] {
		t.Fatalf("current/backups = %v/%v, want one no-backup selection", store.state.Current, store.backups)
	}
	before := cloneState(store.state)
	if _, err := service.SelectLegacy(context.Background(), "missing"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("missing selection error = %v, want ErrProfileNotFound", err)
	}
	if store.state.Current == nil || before.Current == nil || *store.state.Current != *before.Current || len(store.backups) != 1 {
		t.Fatal("missing selection changed current or created a backup")
	}
}

func TestSelectCurrentCreatesBackupAndRejectsDeletedProfile(t *testing.T) {
	current := "old"
	store := &testStore{state: State{
		Current: &current,
		Profiles: map[string]StoredProfile{
			"old": {Profile: provider.Profile{API: "openai-completions"}},
			"new": {Profile: provider.Profile{API: "anthropic-messages"}},
		},
	}}
	service := New(store, &testValidator{})
	if err := service.SelectCurrent(context.Background(), "new"); err != nil {
		t.Fatal(err)
	}
	if store.state.Current == nil || *store.state.Current != "new" || len(store.backups) != 1 || !store.backups[0] {
		t.Fatalf("current/backups = %v/%v, want backed-up selection", store.state.Current, store.backups)
	}
	before := cloneState(store.state)
	if err := service.SelectCurrent(context.Background(), "deleted"); err == nil || err.Error() != `profile "deleted" no longer exists` {
		t.Fatalf("deleted selection error = %v", err)
	}
	if store.state.Current == nil || before.Current == nil || *store.state.Current != *before.Current || len(store.backups) != 1 {
		t.Fatal("deleted selection changed state or created a backup")
	}
}

func TestDeleteIfPresentLegacyIsIdempotentWithoutBackup(t *testing.T) {
	current := "one"
	store := &testStore{state: State{
		Current:  &current,
		Profiles: map[string]StoredProfile{"one": {Profile: provider.Profile{API: "openai-completions"}}},
	}}
	service := New(store, &testValidator{})
	if _, err := service.DeleteIfPresentLegacy(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	if store.state.Current != nil || len(store.state.Profiles) != 0 || len(store.backups) != 1 || store.backups[0] {
		t.Fatalf("delete state/backups = %+v/%v, want empty state and one no-backup transaction", store.state, store.backups)
	}
	calls := store.calls
	if _, err := service.DeleteIfPresentLegacy(context.Background(), "missing"); err != nil {
		t.Fatalf("missing delete = %v, want idempotent success", err)
	}
	if store.calls != calls+1 || len(store.backups) != 2 || store.backups[1] {
		t.Fatalf("missing delete calls/backups = %d/%v, want one no-backup transaction", store.calls, store.backups)
	}
}

func TestSetModelsLegacyValidatesAndDoesNotBackup(t *testing.T) {
	main := "main"
	store := &testStore{state: State{Profiles: map[string]StoredProfile{
		"one": {Profile: provider.Profile{Upstreams: []provider.Upstream{{Name: &main, Models: []provider.ModelEntry{{ID: "old"}}}}}},
	}}}
	service := New(store, &testValidator{})
	models := []provider.ModelEntry{{ID: "new"}}
	if err := service.SetModelsLegacy(context.Background(), "one", "main", models); err != nil {
		t.Fatal(err)
	}
	got := store.state.Profiles["one"].Profile.Upstreams[0].Models
	if len(got) != 1 || got[0].ID != "new" || len(store.backups) != 1 || store.backups[0] {
		t.Fatalf("models/backups = %+v/%v", got, store.backups)
	}
	if err := service.SetModelsLegacy(context.Background(), "one", "missing", models); !errors.Is(err, ErrUnknownChannel) {
		t.Fatalf("unknown channel error = %v", err)
	}
	if err := service.SetModelsLegacy(context.Background(), "one", "main", []provider.ModelEntry{{ID: " "}}); err == nil || err.Error() != "model id must not be empty" {
		t.Fatalf("empty model id error = %v", err)
	}
	if len(store.backups) != 1 || store.state.Profiles["one"].Profile.Upstreams[0].Models[0].ID != "new" {
		t.Fatal("rejected model update changed state or created a backup")
	}
}

func TestNativeDuplicateChecksRevisionAndBacksUp(t *testing.T) {
	current := "source"
	initial := State{
		Current: &current,
		Profiles: map[string]StoredProfile{
			"source": {Profile: provider.Profile{API: "openai-completions"}, Revision: "revision"},
		},
	}
	stale := &testStore{state: cloneState(initial)}
	service := New(stale, &testValidator{})
	if _, err := service.Duplicate(context.Background(), DuplicateCommand{Source: "source", Name: "copy", ExpectedRevision: "stale"}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale duplicate = %v", err)
	}
	if len(stale.backups) != 0 || len(stale.state.Profiles) != 1 {
		t.Fatal("stale duplicate mutated state or created a backup")
	}

	store := &testStore{state: cloneState(initial)}
	service = New(store, &testValidator{})
	result, err := service.Duplicate(context.Background(), DuplicateCommand{Source: "source", Name: " copy ", ExpectedRevision: "revision"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "copy" || store.state.Current == nil || *store.state.Current != "source" || store.state.Profiles["copy"].Profile.API != "openai-completions" || len(store.backups) != 1 || !store.backups[0] {
		t.Fatalf("result/state/backups = %+v/%+v/%v", result, store.state, store.backups)
	}
}

func TestSetExposedModelsLegacyValidatesChannelOwnership(t *testing.T) {
	main := "main"
	store := &testStore{state: State{Profiles: map[string]StoredProfile{
		"one": {Profile: provider.Profile{Upstreams: []provider.Upstream{{Name: &main, Models: []provider.ModelEntry{{ID: "m1"}}}}}},
	}}}
	service := New(store, &testValidator{})
	if err := service.SetExposedModelsLegacy(context.Background(), "one", "main", []string{"m1"}); err != nil {
		t.Fatal(err)
	}
	if got := store.state.Profiles["one"].Profile.Upstreams[0].ExposedModels; len(got) != 1 || got[0] != "m1" {
		t.Fatalf("exposed models = %v", got)
	}
	if err := service.SetExposedModelsLegacy(context.Background(), "one", "main", []string{"unknown"}); err == nil || err.Error() != `exposedModels references unknown model "unknown" in channel "main"` {
		t.Fatalf("unknown exposed model error = %v", err)
	}
	if len(store.backups) != 1 || store.backups[0] {
		t.Fatalf("backups = %v, want only no-backup writes", store.backups)
	}
}

func TestSetUserAgentLegacyValidatesAndClears(t *testing.T) {
	store := &testStore{state: State{Profiles: map[string]StoredProfile{
		"one": {Profile: provider.Profile{}},
	}}}
	service := New(store, &testValidator{})
	codex := "codex"
	if err := service.SetUserAgentLegacy(context.Background(), "one", &codex); err != nil {
		t.Fatal(err)
	}
	if got := store.state.Profiles["one"].Profile.UserAgent; got == nil || *got != codex {
		t.Fatalf("user agent = %v", got)
	}
	invalid := "browser"
	if err := service.SetUserAgentLegacy(context.Background(), "one", &invalid); err == nil || !strings.Contains(err.Error(), `invalid spoof "browser"`) {
		t.Fatalf("invalid spoof error = %v", err)
	}
	if err := service.SetUserAgentLegacy(context.Background(), "one", nil); err != nil {
		t.Fatal(err)
	}
	if store.state.Profiles["one"].Profile.UserAgent != nil || len(store.backups) != 2 || store.backups[0] || store.backups[1] {
		t.Fatalf("user agent/backups = %v/%v", store.state.Profiles["one"].Profile.UserAgent, store.backups)
	}
}

func TestNativeSaveRequiresNameBeforeValidationOrStorage(t *testing.T) {
	store := &testStore{state: State{Profiles: map[string]StoredProfile{}}}
	validator := &testValidator{}
	service := New(store, validator)
	if _, err := service.Save(context.Background(), SaveCommand{Name: "  "}); !errors.Is(err, ErrNameRequired) {
		t.Fatalf("empty name error = %v", err)
	}
	if validator.calls != 0 || store.calls != 0 {
		t.Fatalf("empty name reached validator/store: %d/%d", validator.calls, store.calls)
	}
}

func TestNativeSaveUsesRevisionBackupAndCurrentRename(t *testing.T) {
	current := "source"
	store := &testStore{state: State{
		Current:  &current,
		Profiles: map[string]StoredProfile{"source": {Profile: provider.Profile{API: "old"}, Revision: "expected"}},
	}}
	service := New(store, &testValidator{})
	result, err := service.Save(context.Background(), SaveCommand{
		Original: "source", Name: "renamed", Profile: provider.Profile{API: "new"}, ExpectedRevision: "expected",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "renamed" || store.state.Current == nil || *store.state.Current != "renamed" {
		t.Fatalf("result/current = %+v/%v", result, store.state.Current)
	}
	if _, ok := store.state.Profiles["source"]; ok || len(store.backups) != 1 || !store.backups[0] {
		t.Fatalf("profiles/backups = %+v/%v", store.state.Profiles, store.backups)
	}
}

func TestNativeConflictsDoNotBackupOrMutate(t *testing.T) {
	current := "source"
	initial := State{
		Current: &current,
		Profiles: map[string]StoredProfile{
			"source": {Profile: provider.Profile{API: "source"}, Revision: "actual"},
			"target": {Profile: provider.Profile{API: "target"}, Revision: "target"},
		},
	}
	for _, tc := range []struct {
		name string
		cmd  SaveCommand
		want error
	}{
		{"stale revision", SaveCommand{Original: "source", Name: "renamed", Profile: provider.Profile{API: "replacement"}, ExpectedRevision: "stale"}, ErrRevisionConflict},
		{"existing target", SaveCommand{Original: "source", Name: "target", Profile: provider.Profile{API: "replacement"}, ExpectedRevision: "actual"}, ErrProfileExists},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &testStore{state: cloneState(initial)}
			service := New(store, &testValidator{})
			_, err := service.Save(context.Background(), tc.cmd)
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if len(store.backups) != 0 || store.state.Profiles["source"].Profile.API != "source" || len(store.state.Profiles) != 2 {
				t.Fatalf("conflict mutated state or created backup: %+v", store)
			}
		})
	}
}

func TestNativeDeleteChecksRevisionAndBacksUp(t *testing.T) {
	current := "supplier"
	initial := State{Current: &current, Profiles: map[string]StoredProfile{
		"supplier": {Profile: provider.Profile{API: "openai-completions"}, Revision: "revision"},
	}}
	staleStore := &testStore{state: cloneState(initial)}
	service := New(staleStore, &testValidator{})
	if _, err := service.Delete(context.Background(), DeleteCommand{Name: "supplier", ExpectedRevision: "stale"}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale delete = %v", err)
	}
	if len(staleStore.backups) != 0 || staleStore.state.Current == nil || len(staleStore.state.Profiles) != 1 {
		t.Fatal("stale delete mutated state or created backup")
	}

	store := &testStore{state: cloneState(initial)}
	service = New(store, &testValidator{})
	result, err := service.Delete(context.Background(), DeleteCommand{Name: "supplier", ExpectedRevision: "revision"})
	if err != nil {
		t.Fatal(err)
	}
	if store.state.Current != nil || len(store.state.Profiles) != 0 || len(store.backups) != 1 || !store.backups[0] {
		t.Fatalf("delete result/state/backups = %+v / %+v / %v", result, store.state, store.backups)
	}
}
