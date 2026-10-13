package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domaincatalog "github.com/heihei0299/pi-switch/internal/domain/catalog"
	domainconfig "github.com/heihei0299/pi-switch/internal/domain/config"
	domaingateway "github.com/heihei0299/pi-switch/internal/domain/gateway"
)

type Store interface {
	Read(context.Context) (map[string]interface{}, error)
	Publish(context.Context, domaingateway.CanonicalGatewayPlan) error
}

type Catalog interface {
	Snapshot(context.Context) (domaincatalog.Snapshot, bool, string, error)
}

type EnrichSummary = domaincatalog.EnrichSummary

type SelectedPlan struct {
	Generated domaingateway.CanonicalGatewayPlan
	Plan      domaingateway.CanonicalGatewayPlan
	Selected  []domaingateway.GatewaySelection
}

type Service struct {
	store   Store
	catalog Catalog
}

func New(store Store, catalog Catalog) *Service { return &Service{store: store, catalog: catalog} }

func (s *Service) ReadCurrent(ctx context.Context) (map[string]interface{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, errors.New("gateway store is unavailable")
	}
	return s.store.Read(ctx)
}

func (s *Service) BuildGeneratedPlan(ctx context.Context, cfg domainconfig.PiSwitchConfig, current map[string]interface{}) (domaingateway.CanonicalGatewayPlan, EnrichSummary, error) {
	snapshot, stale, warning, err := s.snapshot(ctx)
	if err != nil {
		return domaingateway.CanonicalGatewayPlan{}, EnrichSummary{}, err
	}
	proposal := domaingateway.BuildProposedGatewayEntry(cfg)
	summary := enrichModels(snapshot, stale, warning, cfg, proposal, domaincatalog.FillOverwrite)
	return domaingateway.BuildGeneratedPlanFromProposal(cfg, current, proposal), summary, nil
}

func (s *Service) BuildDraftPlan(ctx context.Context, cfg domainconfig.PiSwitchConfig, current, draft map[string]interface{}) (domaingateway.CanonicalGatewayPlan, EnrichSummary, error) {
	snapshot, stale, warning, err := s.snapshot(ctx)
	if err != nil {
		return domaingateway.CanonicalGatewayPlan{}, EnrichSummary{}, err
	}
	summary := enrichModels(snapshot, stale, warning, cfg, draft, domaincatalog.FillMissing)
	return domaingateway.BuildDraftPlan(cfg, current, draft), summary, nil
}

func (s *Service) BuildSelectedPlan(ctx context.Context, cfg domainconfig.PiSwitchConfig, current map[string]interface{}, selections []domaingateway.GatewaySelection) (SelectedPlan, EnrichSummary, error) {
	snapshot, stale, warning, err := s.snapshot(ctx)
	if err != nil {
		return SelectedPlan{}, EnrichSummary{}, err
	}
	generatedProposal := domaingateway.BuildProposedGatewayEntry(cfg)
	summary := enrichModels(snapshot, stale, warning, cfg, generatedProposal, domaincatalog.FillOverwrite)
	generated := domaingateway.BuildGeneratedPlanFromProposal(cfg, current, generatedProposal)
	selected := selections
	if selected == nil {
		selected = make([]domaingateway.GatewaySelection, 0)
		for _, group := range generated.Groups {
			for _, model := range group.Models {
				if model.Status == "published" {
					selected = append(selected, domaingateway.GatewaySelection{Supplier: group.Supplier, Channel: group.Channel, Model: model.ID})
				}
			}
		}
	}
	proposed, err := domaingateway.BuildSelectedGatewayEntry(cfg, selected)
	if err != nil {
		return SelectedPlan{}, summary, err
	}
	enrichModels(snapshot, stale, warning, cfg, proposed, domaincatalog.FillMissing)
	return SelectedPlan{Generated: generated, Plan: domaingateway.BuildDraftPlan(cfg, current, proposed), Selected: selected}, summary, nil
}

func (s *Service) EnrichGenerated(ctx context.Context, cfg domainconfig.PiSwitchConfig, proposed map[string]interface{}) (EnrichSummary, error) {
	snapshot, stale, warning, err := s.snapshot(ctx)
	if err != nil {
		return EnrichSummary{}, err
	}
	return enrichModels(snapshot, stale, warning, cfg, proposed, domaincatalog.FillOverwrite), nil
}

func (s *Service) EnrichDraft(ctx context.Context, cfg domainconfig.PiSwitchConfig, draft map[string]interface{}) (EnrichSummary, error) {
	snapshot, stale, warning, err := s.snapshot(ctx)
	if err != nil {
		return EnrichSummary{}, err
	}
	return enrichModels(snapshot, stale, warning, cfg, draft, domaincatalog.FillMissing), nil
}

func (s *Service) Publish(ctx context.Context, plan domaingateway.CanonicalGatewayPlan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(plan.Conflicts) > 0 {
		return fmt.Errorf("gateway plan rejected: %s", strings.Join(plan.Conflicts, "; "))
	}
	if providers, ok := plan.Proposed["providers"].(map[string]interface{}); !ok || providers == nil {
		return fmt.Errorf("gateway.providers is required")
	}
	if plan.PendingCount == 0 {
		return nil
	}
	if s.store == nil {
		return errors.New("gateway store is unavailable")
	}
	return s.store.Publish(ctx, plan)
}

func (s *Service) snapshot(ctx context.Context) (domaincatalog.Snapshot, bool, string, error) {
	if err := ctx.Err(); err != nil {
		return domaincatalog.Snapshot{}, false, "", err
	}
	if s.catalog == nil {
		return domaincatalog.Snapshot{}, false, "models.dev unavailable: catalog is not configured", nil
	}
	return s.catalog.Snapshot(ctx)
}

func enrichModels(snapshot domaincatalog.Snapshot, stale bool, warning string, cfg domainconfig.PiSwitchConfig, target map[string]interface{}, fill func(map[string]interface{}, domaincatalog.Meta) bool) EnrichSummary {
	modelSuppliers := map[string]string{}
	ambiguousModelSuppliers := map[string]bool{}
	for supplier, prof := range cfg.Profiles {
		for _, upstream := range prof.Upstreams {
			for _, id := range upstream.ExposedModels {
				if previous, ok := modelSuppliers[id]; ok && previous != supplier {
					delete(modelSuppliers, id)
					ambiguousModelSuppliers[id] = true
					continue
				}
				if !ambiguousModelSuppliers[id] {
					modelSuppliers[id] = supplier
				}
			}
		}
	}
	enriched, skipped := 0, 0
	providers, ok := target["providers"].(map[string]interface{})
	if !ok {
		return EnrichSummary{Stale: stale, Warning: warning}
	}
	for providerKey, rawProvider := range providers {
		entry, ok := rawProvider.(map[string]interface{})
		if !ok {
			continue
		}
		models, _ := entry["models"].([]interface{})
		supplierHint := providerKey
		if idx := strings.Index(providerKey, "/"); idx > 0 {
			supplierHint = providerKey[:idx]
		}
		for _, rawModel := range models {
			model, ok := rawModel.(map[string]interface{})
			if !ok {
				skipped++
				continue
			}
			id, _ := model["id"].(string)
			supplier := supplierHint
			if providerKey == domaingateway.ResponsesProvider || providerKey == domaingateway.ChatProvider {
				supplier = modelSuppliers[id]
			}
			if id == "" {
				skipped++
				continue
			}
			var meta domaincatalog.Meta
			var found bool
			if supplier != "" {
				if prof, ok := cfg.Profiles[supplier]; ok {
					if providerKey := prof.ModelsDevProviderKey(); providerKey != "" {
						meta, found = snapshot.LookupWithProvider(id, providerKey)
					}
				}
				if !found {
					meta, found = snapshot.LookupWithProvider(id, supplier)
				}
			}
			if !found {
				meta, found = snapshot.Lookup(id)
			}
			if !found {
				skipped++
				continue
			}
			if fill(model, meta) {
				enriched++
			} else {
				skipped++
			}
		}
	}
	return EnrichSummary{Enriched: enriched, Skipped: skipped, Stale: stale, Warning: warning}
}
