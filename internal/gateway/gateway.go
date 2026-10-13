// Package gateway preserves the legacy Gateway entrypoints. Planning lives in
// domain/gateway, while file and catalog IO are adapted by their outbound packages.
package gateway

import (
	"context"

	"github.com/heihei0299/pi-switch/internal/adapters/outbound/filegateway"
	"github.com/heihei0299/pi-switch/internal/bootstrap"
	domainconfig "github.com/heihei0299/pi-switch/internal/domain/config"
	domaingateway "github.com/heihei0299/pi-switch/internal/domain/gateway"
)

const (
	gatewayResponsesProvider = domaingateway.ResponsesProvider
	gatewayChatProvider      = domaingateway.ChatProvider
	gatewayResponsesAPI      = domaingateway.ResponsesAPI
	gatewayChatAPI           = domaingateway.ChatAPI
)

var errGatewayPlanStale = filegateway.ErrStale

type FixedGatewayProvider = domaingateway.FixedGatewayProvider
type GatewayDiagnostic = domaingateway.GatewayDiagnostic
type GatewaySelection = domaingateway.GatewaySelection
type CanonicalGatewayPlan = domaingateway.CanonicalGatewayPlan
type PreviewGroup = domaingateway.PreviewGroup
type PreviewGroupItem = domaingateway.PreviewGroupItem

func ModelsPath() string { return filegateway.ModelsPath() }

func ReadCurrent() (map[string]interface{}, error) {
	return bootstrap.NewGatewayService().ReadCurrent(context.Background())
}

func PublishPlan(plan CanonicalGatewayPlan) error {
	return bootstrap.NewGatewayService().Publish(context.Background(), plan)
}

func IsFixedGatewayProvider(key string) bool        { return domaingateway.IsFixedGatewayProvider(key) }
func FixedGatewayProviders() []FixedGatewayProvider { return domaingateway.FixedGatewayProviders() }
func ValidateGatewayProvider(provider map[string]interface{}) string {
	return domaingateway.ValidateGatewayProvider(provider)
}
func BuildGatewayDiagnostics(cfg domainconfig.PiSwitchConfig) []GatewayDiagnostic {
	return domaingateway.BuildGatewayDiagnostics(cfg)
}
func ValidateProposedGateway(cfg domainconfig.PiSwitchConfig, proposed map[string]interface{}) error {
	return domaingateway.ValidateProposedGateway(cfg, proposed)
}
func BuildProposedGatewayEntry(cfg domainconfig.PiSwitchConfig) map[string]interface{} {
	return domaingateway.BuildProposedGatewayEntry(cfg)
}
func BuildSelectedGatewayEntry(cfg domainconfig.PiSwitchConfig, selections []GatewaySelection) (map[string]interface{}, error) {
	return domaingateway.BuildSelectedGatewayEntry(cfg, selections)
}
func ApplyGatewaySelectionToDraft(cfg domainconfig.PiSwitchConfig, draft map[string]interface{}, selections []GatewaySelection) (map[string]interface{}, error) {
	return domaingateway.ApplyGatewaySelectionToDraft(cfg, draft, selections)
}
func DiffGateway(current, proposed map[string]interface{}) ([]string, []string, []string) {
	return domaingateway.DiffGateway(current, proposed)
}
func ComputePendingCount(current, proposed map[string]interface{}) int {
	return domaingateway.ComputePendingCount(current, proposed)
}
func MergeGatewayExtra(current, proposed map[string]interface{}) map[string]interface{} {
	return domaingateway.MergeGatewayExtra(current, proposed)
}
func MergeGatewayExtraForConfig(cfg domainconfig.PiSwitchConfig, current, proposed map[string]interface{}) map[string]interface{} {
	return domaingateway.MergeGatewayExtraForConfig(cfg, current, proposed)
}
func BuildGeneratedPlan(cfg domainconfig.PiSwitchConfig, current map[string]interface{}) CanonicalGatewayPlan {
	return domaingateway.BuildGeneratedPlan(cfg, current)
}
func BuildDraftPlan(cfg domainconfig.PiSwitchConfig, current, draft map[string]interface{}) CanonicalGatewayPlan {
	return domaingateway.BuildDraftPlan(cfg, current, draft)
}
func BuildPreviewGroups(cfg domainconfig.PiSwitchConfig, current, proposed map[string]interface{}) ([]PreviewGroup, []string) {
	return domaingateway.BuildPreviewGroups(cfg, current, proposed)
}

// These helpers remain package-private only for legacy same-package contract tests.
func gatewayProviderForAPI(api string) string { return domaingateway.ProviderForAPI(api) }
func getProviders(value map[string]interface{}) map[string]interface{} {
	providers, _ := value["providers"].(map[string]interface{})
	return providers
}
