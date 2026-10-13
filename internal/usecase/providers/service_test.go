package providers

import (
	"context"
	"errors"
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
