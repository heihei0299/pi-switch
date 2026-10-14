package providers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/heihei0299/pi-switch/internal/domain/provider"
)

var (
	ErrProfileNotFound    = errors.New("profile not found")
	ErrProfileExists      = errors.New("profile already exists")
	ErrRevisionConflict   = errors.New("configuration changed; reload before saving")
	ErrPersistence        = errors.New("provider configuration persistence failed")
	ErrNameRequired       = errors.New("name required")
	ErrTargetNameRequired = errors.New("as required")
	ErrUnknownChannel     = provider.ErrUnknownChannel
)

type namedProfileError struct {
	name   string
	exists bool
}

func (e namedProfileError) Error() string {
	if e.exists {
		return fmt.Sprintf("target %q already exists", e.name)
	}
	return fmt.Sprintf("profile %q not found", e.name)
}

func (e namedProfileError) Is(target error) bool {
	if e.exists {
		return target == ErrProfileExists
	}
	return target == ErrProfileNotFound
}

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
	ValidateDuplicate(context.Context, provider.Profile) error
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

type DuplicateCommand struct {
	Source           string
	Name             string
	ExpectedRevision string
}

type DeleteLegacyCommand struct{ Name string }

type DeleteCommand struct {
	Name             string
	ExpectedRevision string
}

type Result struct{ Name string }

type ValidationError struct{ Err error }
type DuplicateValidationError struct {
	Err     error
	Profile provider.Profile
}

func (e *ValidationError) Error() string          { return e.Err.Error() }
func (e *ValidationError) Unwrap() error          { return e.Err }
func (e *DuplicateValidationError) Error() string { return e.Err.Error() }
func (e *DuplicateValidationError) Unwrap() error { return e.Err }

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

func (s *Service) CreateLegacy(ctx context.Context, name string, prof provider.Profile) (Result, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Result{}, ErrNameRequired
	}
	if err := s.validate(ctx, prof); err != nil {
		return Result{}, err
	}
	err := s.update(ctx, false, func(state *State) error {
		if state.Profiles == nil {
			state.Profiles = make(map[string]StoredProfile)
		}
		if _, exists := state.Profiles[name]; exists {
			return ErrProfileExists
		}
		state.Profiles[name] = StoredProfile{Profile: prof}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Name: name}, nil
}

func (s *Service) DuplicateLegacy(ctx context.Context, source, name string) (Result, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Result{}, ErrTargetNameRequired
	}
	err := s.update(ctx, false, func(state *State) error {
		profile, ok := state.Profiles[source]
		if !ok {
			return namedProfileError{name: source}
		}
		if _, exists := state.Profiles[name]; exists {
			return namedProfileError{name: name, exists: true}
		}
		if s.validator == nil {
			return &ValidationError{Err: errors.New("provider validator is unavailable")}
		}
		if err := s.validator.ValidateDuplicate(ctx, profile.Profile); err != nil {
			return &ValidationError{Err: err}
		}
		state.Profiles[name] = StoredProfile{Profile: profile.Profile}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Name: name}, nil
}

func (s *Service) Duplicate(ctx context.Context, cmd DuplicateCommand) (Result, error) {
	err := s.update(ctx, true, func(state *State) error {
		source, ok := state.Profiles[cmd.Source]
		if !ok || source.Revision != cmd.ExpectedRevision {
			return ErrRevisionConflict
		}
		name := strings.TrimSpace(cmd.Name)
		if name == "" {
			return &DuplicateValidationError{Err: ErrTargetNameRequired, Profile: source.Profile}
		}
		if _, exists := state.Profiles[name]; exists {
			return &DuplicateValidationError{Err: namedProfileError{name: name, exists: true}, Profile: source.Profile}
		}
		if s.validator == nil {
			return &DuplicateValidationError{Err: errors.New("provider validator is unavailable"), Profile: source.Profile}
		}
		if err := s.validator.ValidateDuplicate(ctx, source.Profile); err != nil {
			return &DuplicateValidationError{Err: err, Profile: source.Profile}
		}
		state.Profiles[name] = StoredProfile{Profile: source.Profile}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Name: strings.TrimSpace(cmd.Name)}, nil
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

// SelectLegacy switches the active profile without a backup, matching the CLI and TUI contract.
func (s *Service) SelectLegacy(ctx context.Context, name string) (Result, error) {
	err := s.update(ctx, false, func(state *State) error {
		if _, ok := state.Profiles[name]; !ok {
			return ErrProfileNotFound
		}
		state.Current = &name
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Name: name}, nil
}

func (s *Service) SelectCurrent(ctx context.Context, name string) error {
	return s.update(ctx, true, func(state *State) error {
		if _, ok := state.Profiles[name]; !ok {
			return &ValidationError{Err: fmt.Errorf("profile %q no longer exists", name)}
		}
		state.Current = &name
		return nil
	})
}

// DeleteIfPresentLegacy keeps the CLI's idempotent delete behavior and always
// completes the same no-backup config transaction, even when the name is absent.
func (s *Service) DeleteIfPresentLegacy(ctx context.Context, name string) (Result, error) {
	err := s.update(ctx, false, func(state *State) error {
		delete(state.Profiles, name)
		if state.Current != nil && *state.Current == name {
			state.Current = nil
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Name: name}, nil
}

func (s *Service) SetModelsLegacy(ctx context.Context, name, channel string, models []provider.ModelEntry) error {
	return s.update(ctx, false, func(state *State) error {
		stored, ok := state.Profiles[name]
		if !ok {
			return ErrProfileNotFound
		}
		if channel == "" {
			return &ValidationError{Err: errors.New("channel is required")}
		}
		profile := stored.Profile
		idx := provider.EnsureMutationChannel(&profile, channel)
		if idx < 0 {
			return fmt.Errorf("%w %q", ErrUnknownChannel, channel)
		}
		seen := make(map[string]bool, len(models))
		for _, model := range models {
			if strings.TrimSpace(model.ID) == "" {
				return &ValidationError{Err: errors.New("model id must not be empty")}
			}
			if seen[model.ID] {
				return &ValidationError{Err: fmt.Errorf("duplicate model id %q", model.ID)}
			}
			seen[model.ID] = true
		}
		profile.Upstreams[idx].Models = models
		state.Profiles[name] = StoredProfile{Profile: profile, Revision: stored.Revision}
		return nil
	})
}

func (s *Service) MergeModelsLegacy(ctx context.Context, name, channel string, entries []provider.ModelEntry) error {
	return s.update(ctx, false, func(state *State) error {
		stored, ok := state.Profiles[name]
		if !ok {
			return ErrProfileNotFound
		}
		profile := stored.Profile
		if err := provider.MergeChannelModels(&profile, channel, entries); err != nil {
			return err
		}
		state.Profiles[name] = StoredProfile{Profile: profile, Revision: stored.Revision}
		return nil
	})
}

func (s *Service) SetExposedModelsLegacy(ctx context.Context, name, channel string, modelIDs []string) error {
	return s.update(ctx, false, func(state *State) error {
		stored, ok := state.Profiles[name]
		if !ok {
			return ErrProfileNotFound
		}
		if channel == "" {
			return &ValidationError{Err: errors.New("channel is required")}
		}
		profile := stored.Profile
		idx := provider.EnsureMutationChannel(&profile, channel)
		if idx < 0 {
			return fmt.Errorf("%w %q", ErrUnknownChannel, channel)
		}
		models := make(map[string]bool, len(profile.Upstreams[idx].Models))
		for _, model := range profile.Upstreams[idx].Models {
			models[model.ID] = true
		}
		for _, id := range modelIDs {
			if !models[id] {
				return &ValidationError{Err: fmt.Errorf("exposedModels references unknown model %q in channel %q", id, channel)}
			}
		}
		profile.Upstreams[idx].ExposedModels = modelIDs
		state.Profiles[name] = StoredProfile{Profile: profile, Revision: stored.Revision}
		return nil
	})
}

func (s *Service) SetUserAgentLegacy(ctx context.Context, name string, spoof *string) error {
	return s.update(ctx, false, func(state *State) error {
		stored, ok := state.Profiles[name]
		if !ok {
			return ErrProfileNotFound
		}
		profile := stored.Profile
		profile.UserAgent = nil
		if spoof != nil && *spoof != "" {
			switch *spoof {
			case "claude-code", "codex", "gemini":
				value := *spoof
				profile.UserAgent = &value
			default:
				return &ValidationError{Err: fmt.Errorf("invalid spoof %q, must be one of '', 'claude-code', 'codex', 'gemini'", *spoof)}
			}
		}
		state.Profiles[name] = StoredProfile{Profile: profile, Revision: stored.Revision}
		return nil
	})
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
		var validation *ValidationError
		var duplicateValidation *DuplicateValidationError
		if errors.Is(err, ErrProfileNotFound) || errors.Is(err, ErrProfileExists) || errors.Is(err, ErrRevisionConflict) || errors.Is(err, ErrUnknownChannel) || errors.As(err, &validation) || errors.As(err, &duplicateValidation) {
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
