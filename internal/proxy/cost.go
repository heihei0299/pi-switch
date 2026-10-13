package proxy

import (
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/domain/usage"
)

func CalcCost(entry *config.ModelEntry, prompt, completion, cached int) *float64 {
	return usage.CalcCost(entry, prompt, completion, cached)
}

func CalcUsageCost(entry *config.ModelEntry, summary *usage.UsageSummary) *float64 {
	return usage.CalcUsageCost(entry, summary)
}
