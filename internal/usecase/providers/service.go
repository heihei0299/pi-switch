package providers

import (
	"context"
	"errors"
	"strings"

	"github.com/heihei0299/pi-switch/internal/domain/provider"
)

var (
	ErrProfileNotFound  = errors.New("profile not found")
	ErrProfileExists    = errors.New("profile already exists")
	ErrRevisionConflict = errors.New("configuration changed; reload before saving")
	ErrPersistence      = errors.New("provider configuration persistence failed")
	ErrNameRequired     = errors.New("name required")
)

type StoredProfile struct {
	Profile  provider.Profile
	Revision string
}

type State struct {
	Current  *string
	Profiles map[string]StoredProfile
}

type Store interface {
	Update(context.Context, bool, func(*State) error) error
}

type Validator interface {
	Validate(context.Context, provider.Profile) error
}

type Service struct {
	store     Store
	validator Validator
}

func New(store Store, validator Validator) *Service {
	return &Service{store: store, validator: validator}
}

type PutLegacyCommand struct {
	Name       string
	RenameFrom string
	Profile    provider.Profile
}

type SaveCommand struct {
	Original         string
	Name             string
	Profile          provider.Profile
	ExpectedRevision string
}

type DeleteLegacyCommand struct{ Name string }

type DeleteCommand struct {
	Name             string
	ExpectedRevision string
}

type Result struct{ Name string }

type ValidationError struct{ Err error }

func (e *ValidationError) Error() string { return e.Err.Error() }
func (e *ValidationError) Unwrap() error { return e.Err }

type PersistenceError struct{ Err error }

func (e *PersistenceError) Error() string        { return e.Err.Error() }
func (e *PersistenceError) Unwrap() error        { return e.Err }
func (e *PersistenceError) Is(target error) bool { return target == ErrPersistence }

func (s *Service) PutLegacy(ctx context.Context, cmd PutLegacyCommand) (Result, error) {
	if err := s.validate(ctx, cmd.Profile); err != nil {
		return Result{}, err
	}
	err := s.update(ctx, false, func(state *State) error {
		if state.Profiles == nil {
			state.Profiles = make(map[string]StoredProfile)
		}
		if cmd.RenameFrom != "" && cmd.RenameFrom != cmd.Name {
			delete(state.Profiles, cmd.RenameFrom)
		}
		state.Profiles[cmd.Name] = StoredProfile{Profile: cmd.Profile}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Name: cmd.Name}, nil
}

func (s *Service) Save(ctx context.Context, cmd SaveCommand) (Result, error) {
	name := strings.TrimSpace(cmd.Name)
	if name == "" {
		return Result{}, ErrNameRequired
	}
	if err := s.validate(ctx, cmd.Profile); err != nil {
		return Result{}, err
	}
	err := s.update(ctx, true, func(state *State) error {
		if cmd.Original != "" {
			old, ok := state.Profiles[cmd.Original]
			if !ok || old.Revision != cmd.ExpectedRevision {
				return ErrRevisionConflict
			}
		}
		if _, exists := state.Profiles[name]; exists && name != cmd.Original {
			return ErrProfileExists
		}
		if state.Profiles == nil {
			state.Profiles = make(map[string]StoredProfile)
		}
		if cmd.Original != name {
			delete(state.Profiles, cmd.Original)
			if state.Current != nil && *state.Current == cmd.Original {
				state.Current = &name
			}
		}
		state.Profiles[name] = StoredProfile{Profile: cmd.Profile}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Name: name}, nil
}

func (s *Service) DeleteLegacy(ctx context.Context, cmd DeleteLegacyCommand) (Result, error) {
	err := s.update(ctx, false, func(state *State) error {
		if _, ok := state.Profiles[cmd.Name]; !ok {
			return ErrProfileNotFound
		}
		delete(state.Profiles, cmd.Name)
		if state.Current != nil && *state.Current == cmd.Name {
			state.Current = nil
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Name: cmd.Name}, nil
}

func (s *Service) Delete(ctx context.Context, cmd DeleteCommand) (Result, error) {
	err := s.update(ctx, true, func(state *State) error {
		profile, ok := state.Profiles[cmd.Name]
		if !ok || profile.Revision != cmd.ExpectedRevision {
			return ErrRevisionConflict
		}
		delete(state.Profiles, cmd.Name)
		if state.Current != nil && *state.Current == cmd.Name {
			state.Current = nil
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Name: cmd.Name}, nil
}

func (s *Service) validate(ctx context.Context, profile provider.Profile) error {
	if s.validator == nil {
		return &ValidationError{Err: errors.New("provider validator is unavailable")}
	}
	if err := s.validator.Validate(ctx, profile); err != nil {
		return &ValidationError{Err: err}
	}
	return nil
}

func (s *Service) update(ctx context.Context, backup bool, mutate func(*State) error) error {
	if s.store == nil {
		return &PersistenceError{Err: errors.New("provider store is unavailable")}
	}
	if err := s.store.Update(ctx, backup, mutate); err != nil {
		if errors.Is(err, ErrProfileNotFound) || errors.Is(err, ErrProfileExists) || errors.Is(err, ErrRevisionConflict) {
			return err
		}
		var persistence *PersistenceError
		if errors.As(err, &persistence) {
			return err
		}
		return &PersistenceError{Err: err}
	}
	return nil
}
