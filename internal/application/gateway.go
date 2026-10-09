package application

import (
	"github.com/heihei0299/pi-switch/internal/catalog"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/gateway"
)

// BuildGeneratedGatewayPlan is the shared generated Gateway use case.
func BuildGeneratedGatewayPlan(cfg config.PiSwitchConfig, current map[string]interface{}) (gateway.CanonicalGatewayPlan, catalog.EnrichSummary) {
	return gateway.BuildEnrichedGeneratedPlan(cfg, current)
}

// BuildDraftGatewayPlan is the shared draft Gateway use case. Draft metadata is
// filled only when absent; explicit draft values remain authoritative.
func BuildDraftGatewayPlan(cfg config.PiSwitchConfig, current, draft map[string]interface{}) (gateway.CanonicalGatewayPlan, catalog.EnrichSummary) {
	summary := gateway.EnrichDraftModels(cfg, draft)
	return gateway.BuildDraftPlan(cfg, current, draft), summary
}

// SelectedGatewayPlan contains the generated candidate groups and the plan for
// the selected models, so Native and HTTP callers can share the same rules.
type SelectedGatewayPlan struct {
	Generated gateway.CanonicalGatewayPlan
	Plan      gateway.CanonicalGatewayPlan
	Selected  []gateway.GatewaySelection
}

// BuildSelectedGatewayPlan builds the selection preview used by the Native UI.
// A nil selection means the currently published Gateway models remain selected.
func BuildSelectedGatewayPlan(cfg config.PiSwitchConfig, current map[string]interface{}, selections []gateway.GatewaySelection) (SelectedGatewayPlan, error) {
	generated, _ := BuildGeneratedGatewayPlan(cfg, current)
	selected := selections
	if selected == nil {
		selected = make([]gateway.GatewaySelection, 0)
		for _, group := range generated.Groups {
			for _, model := range group.Models {
				if model.Status == "published" {
					selected = append(selected, gateway.GatewaySelection{Supplier: group.Supplier, Channel: group.Channel, Model: model.ID})
				}
			}
		}
	}
	proposed, err := gateway.BuildSelectedGatewayEntry(cfg, selected)
	if err != nil {
		return SelectedGatewayPlan{}, err
	}
	gateway.EnrichDraftModels(cfg, proposed)
	return SelectedGatewayPlan{
		Generated: generated,
		Plan:      gateway.BuildDraftPlan(cfg, current, proposed),
		Selected:  selected,
	}, nil
}

func PublishGatewayPlan(plan gateway.CanonicalGatewayPlan) error {
	return gateway.PublishPlan(plan)
}
