package proxy

import "github.com/heihei0299/pi-switch/internal/config"

// CalcCost computes cost stub: (prompt-cached)*input + cached*cacheRead + completion*output divided by 1M.
// Returns nil when ModelEntry has no cost (unknown).
func CalcCost(m config.ModelEntry, prompt, completion, cached int) *float64 {
	if m.Cost == nil {
		return nil
	}
	// Rates are per 1M tokens.
	input := m.Cost.Input
	output := m.Cost.Output
	cacheRead := m.Cost.CacheRead
	// When rates are zero and not set, still treat as valid (cost may be 0); only nil cost yields unknown.
	nonCached := prompt - cached
	if nonCached < 0 {
		nonCached = 0
	}
	cost := float64(nonCached)*input/1_000_000 + float64(cached)*cacheRead/1_000_000 + float64(completion)*output/1_000_000
	return &cost
}
