package gateway

import (
	"strings"

	"github.com/heihei0299/pi-switch/internal/catalog"
	"github.com/heihei0299/pi-switch/internal/config"
)

// BuildEnrichedGeneratedPlan is the one generated flow every entry point uses:
// build the config-derived proposal, enrich it from the models.dev snapshot, and
// compute the canonical plan from that exact enriched proposal. The summary is
// returned so preview surfaces report the same enrich counters as the plan.
func BuildEnrichedGeneratedPlan(cfg config.PiSwitchConfig, current map[string]interface{}) (CanonicalGatewayPlan, catalog.EnrichSummary) {
	proposal := BuildProposedGatewayEntry(cfg)
	summary := EnrichProposedModels(cfg, proposal)
	return buildGeneratedPlanFromProposal(cfg, current, proposal), summary
}

// EnrichProposedModels fills gateway proposed models from the models.dev snapshot.
// It prefers provider/bare lookup using the supplier's resolved models.dev provider
// (or supplier name as hint) to disambiguate duplicate bare ids, and overwrites
// stale defaults (e.g. 128000→1048576) when the catalog provides non-zero values.
//
// It is exported so preview and publish paths share one implementation instead of
// each adapter re-deriving the lookup rules.
func EnrichProposedModels(cfg config.PiSwitchConfig, proposed map[string]interface{}) catalog.EnrichSummary {
	snap, stale, warning := catalog.Ensure()
	enriched, skipped := 0, 0
	// Fixed gateway providers use bare ids; resolve catalog metadata through the originating supplier.
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
	provs, ok := proposed["providers"].(map[string]interface{})
	if !ok {
		return catalog.EnrichSummary{Enriched: enriched, Skipped: skipped, Stale: stale, Warning: warning}
	}
	for providerKey, pv := range provs {
		entry, ok := pv.(map[string]interface{})
		if !ok {
			continue
		}
		models, _ := entry["models"].([]interface{})
		// Non-fixed provider entries use the provider key as their supplier hint.
		supplierHint := providerKey
		if idx := strings.Index(providerKey, "/"); idx > 0 {
			supplierHint = providerKey[:idx]
		}
		for _, m := range models {
			mm, ok := m.(map[string]interface{})
			if !ok {
				skipped++
				continue
			}
			id, _ := mm["id"].(string)
			supplier := supplierHint
			if providerKey == gatewayResponsesProvider || providerKey == gatewayChatProvider {
				supplier = modelSuppliers[id]
			}
			if id == "" {
				skipped++
				continue
			}
			var meta catalog.Meta
			var found bool
			if supplier != "" {
				if prof, ok := cfg.Profiles[supplier]; ok {
					if pk := prof.ModelsDevProviderKey(); pk != "" {
						meta, found = snap.LookupWithProvider(id, pk)
					}
				}
				if !found {
					meta, found = snap.LookupWithProvider(id, supplier)
				}
			}
			if !found {
				meta, found = snap.Lookup(id)
			}
			if !found {
				skipped++
				continue
			}
			if catalog.FillOverwrite(mm, meta) {
				enriched++
			} else {
				skipped++
			}
		}
	}
	return catalog.EnrichSummary{Enriched: enriched, Skipped: skipped, Stale: stale, Warning: warning}
}
