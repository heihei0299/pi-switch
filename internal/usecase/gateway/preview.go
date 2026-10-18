package gateway

import (
	"context"
	"errors"

	domainconfig "github.com/heihei0299/pi-switch/internal/domain/config"
	domaingateway "github.com/heihei0299/pi-switch/internal/domain/gateway"
)

type ConfigReader interface {
	Load() (domainconfig.PiSwitchConfig, string, error)
}

// Preview keeps the configuration used for planning private, except for the
// proxy host needed by the existing publish authentication warning.
type Preview struct {
	Plan      domaingateway.CanonicalGatewayPlan
	Enrich    EnrichSummary
	ProxyHost string
}

type ConfigReadError struct{ Err error }

func (e *ConfigReadError) Error() string { return e.Err.Error() }
func (e *ConfigReadError) Unwrap() error { return e.Err }

type SelectionError struct{ Err error }

func (e *SelectionError) Error() string { return e.Err.Error() }
func (e *SelectionError) Unwrap() error { return e.Err }

func (s *Service) loadConfig(ctx context.Context) (domainconfig.PiSwitchConfig, error) {
	if err := ctx.Err(); err != nil {
		return domainconfig.PiSwitchConfig{}, err
	}
	if s.config == nil {
		return domainconfig.PiSwitchConfig{}, &ConfigReadError{Err: errors.New("gateway configuration is unavailable")}
	}
	cfg, _, err := s.config.Load()
	if err != nil {
		return domainconfig.PiSwitchConfig{}, &ConfigReadError{Err: err}
	}
	return cfg, ctx.Err()
}

func (s *Service) previewState(ctx context.Context) (domainconfig.PiSwitchConfig, map[string]interface{}, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return domainconfig.PiSwitchConfig{}, nil, err
	}
	current, err := s.ReadCurrent(ctx)
	return cfg, current, err
}

func (s *Service) PreviewGenerated(ctx context.Context) (Preview, error) {
	cfg, current, err := s.previewState(ctx)
	if err != nil {
		return Preview{}, err
	}
	plan, summary, err := s.BuildGeneratedPlan(ctx, cfg, current)
	return Preview{Plan: plan, Enrich: summary, ProxyHost: cfg.Settings.Proxy.Host}, err
}

func (s *Service) PreviewDraft(ctx context.Context, draft map[string]interface{}) (Preview, error) {
	cfg, current, err := s.previewState(ctx)
	if err != nil {
		return Preview{}, err
	}
	plan, summary, err := s.BuildDraftPlan(ctx, cfg, current, draft)
	return Preview{Plan: plan, Enrich: summary, ProxyHost: cfg.Settings.Proxy.Host}, err
}

func (s *Service) PreviewSelected(ctx context.Context, selections []domaingateway.GatewaySelection) (SelectedPlan, EnrichSummary, error) {
	cfg, current, err := s.previewState(ctx)
	if err != nil {
		return SelectedPlan{}, EnrichSummary{}, err
	}
	return s.buildSelectedPlan(ctx, cfg, current, selections)
}

// PreviewSelectedDraft preserves HTTP's fill-missing draft contract. Native's
// candidate preview uses PreviewSelected, including generated catalog metadata.
func (s *Service) PreviewSelectedDraft(ctx context.Context, draft map[string]interface{}, selections []domaingateway.GatewaySelection) (Preview, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return Preview{}, err
	}
	var edited map[string]interface{}
	if draft != nil {
		edited, err = domaingateway.ApplyGatewaySelectionToDraft(cfg, draft, selections)
	} else {
		edited, err = domaingateway.BuildSelectedGatewayEntry(cfg, selections)
	}
	if err != nil {
		return Preview{}, &SelectionError{Err: err}
	}
	current, err := s.ReadCurrent(ctx)
	if err != nil {
		return Preview{}, err
	}
	plan, summary, err := s.BuildDraftPlan(ctx, cfg, current, edited)
	return Preview{Plan: plan, Enrich: summary, ProxyHost: cfg.Settings.Proxy.Host}, err
}
