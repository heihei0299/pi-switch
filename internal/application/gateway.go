package application

import (
	"context"

	"github.com/heihei0299/pi-switch/internal/bootstrap"
	"github.com/heihei0299/pi-switch/internal/catalog"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/gateway"
	gatewayusecase "github.com/heihei0299/pi-switch/internal/usecase/gateway"
)

func ReadCurrentGatewayContext(ctx context.Context) (map[string]interface{}, error) {
	return bootstrap.NewGatewayService().ReadCurrent(ctx)
}

func ReadCurrentGateway() (map[string]interface{}, error) {
	return ReadCurrentGatewayContext(context.Background())
}

// BuildGeneratedGatewayPlanContext returns the exact enriched plan used by publish.
func BuildGeneratedGatewayPlanContext(ctx context.Context, cfg config.PiSwitchConfig, current map[string]interface{}) (gateway.CanonicalGatewayPlan, catalog.EnrichSummary, error) {
	return bootstrap.NewGatewayService().BuildGeneratedPlan(ctx, cfg, current)
}

func BuildGeneratedGatewayPlan(cfg config.PiSwitchConfig, current map[string]interface{}) (gateway.CanonicalGatewayPlan, catalog.EnrichSummary) {
	plan, summary, _ := BuildGeneratedGatewayPlanContext(context.Background(), cfg, current)
	return plan, summary
}

// BuildDraftGatewayPlanContext leaves stated draft metadata authoritative.
func BuildDraftGatewayPlanContext(ctx context.Context, cfg config.PiSwitchConfig, current, draft map[string]interface{}) (gateway.CanonicalGatewayPlan, catalog.EnrichSummary, error) {
	return bootstrap.NewGatewayService().BuildDraftPlan(ctx, cfg, current, draft)
}

func BuildDraftGatewayPlan(cfg config.PiSwitchConfig, current, draft map[string]interface{}) (gateway.CanonicalGatewayPlan, catalog.EnrichSummary) {
	plan, summary, _ := BuildDraftGatewayPlanContext(context.Background(), cfg, current, draft)
	return plan, summary
}

// SelectedGatewayPlan is the result used by the Native selection page.
type SelectedGatewayPlan = gatewayusecase.SelectedPlan

func BuildSelectedGatewayPlanContext(ctx context.Context, cfg config.PiSwitchConfig, current map[string]interface{}, selections []gateway.GatewaySelection) (SelectedGatewayPlan, error) {
	selected, _, err := bootstrap.NewGatewayService().BuildSelectedPlan(ctx, cfg, current, selections)
	return selected, err
}

func BuildSelectedGatewayPlan(cfg config.PiSwitchConfig, current map[string]interface{}, selections []gateway.GatewaySelection) (SelectedGatewayPlan, error) {
	return BuildSelectedGatewayPlanContext(context.Background(), cfg, current, selections)
}

func BuildProposedGatewayEntry(cfg config.PiSwitchConfig) map[string]interface{} {
	return gateway.BuildProposedGatewayEntry(cfg)
}

func BuildSelectedGatewayEntry(cfg config.PiSwitchConfig, selections []gateway.GatewaySelection) (map[string]interface{}, error) {
	return gateway.BuildSelectedGatewayEntry(cfg, selections)
}

func ApplyGatewaySelectionToDraft(cfg config.PiSwitchConfig, draft map[string]interface{}, selections []gateway.GatewaySelection) (map[string]interface{}, error) {
	return gateway.ApplyGatewaySelectionToDraft(cfg, draft, selections)
}

func PublishGatewayPlanContext(ctx context.Context, plan gateway.CanonicalGatewayPlan) error {
	return bootstrap.NewGatewayService().Publish(ctx, plan)
}

func PublishGatewayPlan(plan gateway.CanonicalGatewayPlan) error {
	return PublishGatewayPlanContext(context.Background(), plan)
}
