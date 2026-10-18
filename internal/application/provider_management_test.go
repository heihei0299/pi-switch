package application

import (
	"context"
	"errors"
	"testing"

	domainprovider "github.com/heihei0299/pi-switch/internal/domain/provider"
	providerusecase "github.com/heihei0299/pi-switch/internal/usecase/providers"
)

type injectedProviderStore struct {
	state  providerusecase.State
	err    error
	ctx    context.Context
	calls  int
	backup bool
}

func (s *injectedProviderStore) Update(ctx context.Context, backup bool, mutate func(*providerusecase.State) error) error {
	s.ctx, s.backup = ctx, backup
	s.calls++
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.err != nil {
		return s.err
	}
	return mutate(&s.state)
}

type injectedProviderValidator struct{ err error }

func (v injectedProviderValidator) Validate(context.Context, domainprovider.Profile) error {
	return v.err
}
func (v injectedProviderValidator) ValidateDuplicate(context.Context, domainprovider.Profile) error {
	return v.err
}

func TestProviderManagementInjectsUsecaseAndPreservesWriteErrors(t *testing.T) {
	wantErr := errors.New("injected persistence failure")
	profile := domainprovider.Profile{API: "openai-chat"}
	for _, operation := range []string{"save", "delete", "select", "duplicate"} {
		t.Run(operation, func(t *testing.T) {
			store := &injectedProviderStore{err: wantErr, state: providerusecase.State{
				Current:  managementStringPointer("source"),
				Profiles: map[string]providerusecase.StoredProfile{"source": {Profile: profile}},
			}}
			service := providerusecase.New(store, injectedProviderValidator{})
			ctx := context.WithValue(context.Background(), struct{}{}, operation)
			var err error
			switch operation {
			case "save":
				err = SaveProvider(ctx, service, "", "copy", profile, "")
			case "delete":
				err = DeleteProvider(ctx, service, "source", "revision")
			case "select":
				err = SetCurrentProfile(ctx, service, "source")
			case "duplicate":
				err = DuplicateProvider(ctx, service, "source", "copy", "revision")
			}
			if !errors.Is(err, wantErr) || store.calls != 1 || store.ctx.Value(struct{}{}) != operation || !store.backup {
				t.Fatalf("error=%v calls=%d context=%v backup=%v", err, store.calls, store.ctx, store.backup)
			}
		})
	}
}

func TestProviderManagementCanceledContextDoesNotWrite(t *testing.T) {
	store := &injectedProviderStore{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := DeleteProvider(ctx, providerusecase.New(store, injectedProviderValidator{}), "source", "revision")
	if !errors.Is(err, context.Canceled) || store.calls != 0 {
		t.Fatalf("error=%v store calls=%d", err, store.calls)
	}
}

func managementStringPointer(value string) *string { return &value }

func TestProviderManagementValidationPreservesCancellation(t *testing.T) {
	for _, wantErr := range []error{context.Canceled, context.DeadlineExceeded} {
		for _, duplicate := range []bool{false, true} {
			store := &injectedProviderStore{state: providerusecase.State{Profiles: map[string]providerusecase.StoredProfile{"source": {Revision: "revision"}}}}
			service := providerusecase.New(store, injectedProviderValidator{err: wantErr})
			var err error
			if duplicate {
				err = DuplicateProvider(context.Background(), service, "source", "copy", "revision")
			} else {
				err = SaveProvider(context.Background(), service, "source", "renamed", domainprovider.Profile{}, "revision")
			}
			if !errors.Is(err, wantErr) || len(store.state.Profiles) != 1 || store.state.Profiles["source"].Revision != "revision" {
				t.Fatalf("duplicate=%v error=%v, want %v without changing state", duplicate, err, wantErr)
			}
		}
	}
}

func TestProviderManagementInMemoryContracts(t *testing.T) {
	for _, operation := range []string{"save", "delete", "select", "duplicate"} {
		for _, stale := range []bool{false, true} {
			store := &injectedProviderStore{state: providerusecase.State{
				Current:  managementStringPointer("source"),
				Profiles: map[string]providerusecase.StoredProfile{"source": {Revision: "revision"}},
			}}
			service := providerusecase.New(store, injectedProviderValidator{})
			expected := "revision"
			if stale {
				expected = "stale"
			}
			ctx := context.Background()
			var err error
			switch operation {
			case "save":
				err = SaveProvider(ctx, service, "source", "renamed", domainprovider.Profile{}, expected)
			case "delete":
				err = DeleteProvider(ctx, service, "source", expected)
			case "select":
				err = SetCurrentProfile(ctx, service, "source")
			case "duplicate":
				err = DuplicateProvider(ctx, service, "source", "copy", expected)
			}
			if stale && operation != "select" {
				if !errors.Is(err, ErrManagementConflict) || len(store.state.Profiles) != 1 || *store.state.Current != "source" {
					t.Fatalf("%s stale error=%v state=%+v", operation, err, store.state)
				}
				continue
			}
			if err != nil {
				t.Fatalf("%s: %v", operation, err)
			}
			switch operation {
			case "save":
				if _, ok := store.state.Profiles["renamed"]; !ok || *store.state.Current != "renamed" {
					t.Fatal("rename did not update current")
				}
			case "delete":
				if len(store.state.Profiles) != 0 || store.state.Current != nil {
					t.Fatal("delete left current or profile")
				}
			case "select":
				if *store.state.Current != "source" {
					t.Fatal("selection not stored")
				}
			case "duplicate":
				if _, ok := store.state.Profiles["copy"]; !ok || len(store.state.Profiles) != 2 || *store.state.Current != "source" {
					t.Fatal("duplicate changed current or lost profiles")
				}
			}
		}
	}
}
