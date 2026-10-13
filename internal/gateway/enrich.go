package gateway

import (
	"context"

	"github.com/heihei0299/pi-switch/internal/bootstrap"
	"github.com/heihei0299/pi-switch/internal/catalog"
	"github.com/heihei0299/pi-switch/internal/config"
)

func BuildEnrichedGeneratedPlan(cfg config.PiSwitchConfig, current map[string]interface{}) (CanonicalGatewayPlan, catalog.EnrichSummary) {
	plan, summary, err := bootstrap.NewGatewayService().BuildGeneratedPlan(context.Background(), cfg, current)
	if err != nil {
		return CanonicalGatewayPlan{}, summary
	}
	return plan, summary
}

func EnrichProposedModels(cfg config.PiSwitchConfig, proposed map[string]interface{}) catalog.EnrichSummary {
	summary, _ := bootstrap.NewGatewayService().EnrichGenerated(context.Background(), cfg, proposed)
	return summary
}

func EnrichDraftModels(cfg config.PiSwitchConfig, draft map[string]interface{}) catalog.EnrichSummary {
	summary, _ := bootstrap.NewGatewayService().EnrichDraft(context.Background(), cfg, draft)
	return summary
}
