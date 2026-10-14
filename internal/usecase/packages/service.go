package packages

import (
	"errors"
	"path/filepath"
	"strings"
	"time"
)

const (
	StatusImported = "imported"
	StatusEmpty    = "empty"
	StatusNotFound = "not_found"
)

var ErrPackageNotFound = errors.New("package not found")

type Record struct {
	ID            string
	Spec          string
	Type          string
	Name          string
	Version       string
	Description   string
	Homepage      string
	HasExtensions bool
	HasSkills     bool
	HasPrompts    bool
	HasThemes     bool
	Installed     bool
	Enabled       bool
	InstalledAt   *int64
	Origin        string
	SourcePath    string
	Manifest      string
}

type PackageDTO struct {
	ID            string `json:"id"`
	Spec          string `json:"spec"`
	Type          string `json:"type"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	Description   string `json:"description"`
	Homepage      string `json:"homepage"`
	Origin        string `json:"origin"`
	HasExtensions bool   `json:"hasExtensions"`
	HasSkills     bool   `json:"hasSkills"`
	HasPrompts    bool   `json:"hasPrompts"`
	HasThemes     bool   `json:"hasThemes"`
	Installed     bool   `json:"installed"`
	Enabled       bool   `json:"enabled"`
	InstalledAt   string `json:"installedAt,omitempty"`
}

type ImportedPackage struct {
	ID            string
	Spec          string
	Type          string
	Name          string
	Version       string
	Description   string
	Homepage      string
	HasExtensions bool
	HasSkills     bool
	HasPrompts    bool
	HasThemes     bool
	Enabled       bool
	SourcePath    string
	Manifest      string
}

type ImportResult struct {
	OK         bool              `json:"ok"`
	Count      int               `json:"count"`
	Discovered int               `json:"discovered"`
	Skipped    int               `json:"skipped"`
	Status     string            `json:"status"`
	Message    string            `json:"message"`
	Warnings   []string          `json:"warnings,omitempty"`
	Packages   []ImportedPackage `json:"-"`
}

type AddCommand struct {
	Spec    string
	Enabled bool
}

type Repository interface {
	ListInstalled() ([]Record, error)
	GetInstalled(id string) (Record, error)
	Add(record Record) error
	DeleteInstalled(id string) error
	ToggleInstalled(id string) (bool, error)
	SyncImported(packages []ImportedPackage) error
}

type Discoverer interface {
	DiscoverPackages() (ImportResult, error)
}

type Service struct {
	repository Repository
	discoverer Discoverer
}

func NewService(repository Repository, discoverer Discoverer) Service {
	return Service{repository: repository, discoverer: discoverer}
}

func (s Service) ListInstalled() ([]PackageDTO, error) {
	records, err := s.repository.ListInstalled()
	if err != nil {
		return nil, err
	}
	out := make([]PackageDTO, 0, len(records))
	for _, record := range records {
		out = append(out, packageDTO(record, true))
	}
	return out, nil
}

func (s Service) Add(command AddCommand) error {
	spec := strings.TrimSpace(command.Spec)
	if spec == "" {
		return errors.New("spec required")
	}
	typ, name := parsePackageSpec(spec)
	return s.repository.Add(Record{ID: spec, Spec: spec, Type: typ, Name: name, Installed: true, Enabled: command.Enabled, Origin: "manual"})
}

func (s Service) GetInstalled(id string) (PackageDTO, error) {
	record, err := s.repository.GetInstalled(id)
	if err != nil {
		return PackageDTO{}, err
	}
	return packageDTO(record, false), nil
}

func (s Service) DeleteInstalled(id string) error {
	if id == "" {
		return errors.New("package id is required")
	}
	return s.repository.DeleteInstalled(id)
}

func (s Service) ToggleInstalled(id string) (bool, error) {
	if id == "" {
		return false, errors.New("package id is required")
	}
	return s.repository.ToggleInstalled(id)
}

func (s Service) ImportPiPackages() (ImportResult, error) {
	result, err := s.discoverer.DiscoverPackages()
	if err != nil {
		return ImportResult{}, err
	}
	if result.Status == StatusNotFound {
		return result, nil
	}
	if err := s.repository.SyncImported(result.Packages); err != nil {
		return ImportResult{}, err
	}
	return result, nil
}

func packageDTO(record Record, nano bool) PackageDTO {
	out := PackageDTO{
		ID: record.ID, Spec: record.Spec, Type: record.Type, Name: record.Name,
		Version: record.Version, Description: record.Description, Homepage: record.Homepage, Origin: record.Origin,
		HasExtensions: record.HasExtensions, HasSkills: record.HasSkills, HasPrompts: record.HasPrompts, HasThemes: record.HasThemes,
		Installed: record.Installed, Enabled: record.Enabled,
	}
	if record.InstalledAt != nil && *record.InstalledAt != 0 {
		value := *record.InstalledAt
		var installedAt time.Time
		if value > 1e12 {
			installedAt = time.Unix(value/1000, value%1000*int64(time.Millisecond))
		} else {
			installedAt = time.Unix(value, 0)
		}
		layout := time.RFC3339
		if nano {
			layout = time.RFC3339Nano
		}
		out.InstalledAt = installedAt.Format(layout)
	}
	return out
}

func parsePackageSpec(spec string) (typ, name string) {
	if idx := strings.Index(spec, ":"); idx >= 0 {
		typ = spec[:idx]
		name = spec[idx+1:]
		if typ == "" {
			typ = "npm"
		}
		if name == "" {
			name = spec
		}
		return typ, name
	}
	if strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") {
		name = filepath.Base(spec)
		if name == "." || name == "" {
			name = spec
		}
		return "local", name
	}
	return "npm", spec
}
