package proxy

import "github.com/heihei0299/pi-switch/internal/config"

// CalcCost is the single implementation of request cost:
// (prompt-cached)*input + cached*cacheRead + completion*output, divided by 1M.
// Returns nil when entry is nil or has no cost (unknown).
func CalcCost(entry *config.ModelEntry, prompt, completion, cached int) *float64 {
	if entry == nil || entry.Cost == nil {
		return nil
	}
	// Rates are per 1M tokens.
	input := entry.Cost.Input
	output := entry.Cost.Output
	cacheRead := entry.Cost.CacheRead
	// When rates are zero and not set, still treat as valid (cost may be 0); only nil cost yields unknown.
	nonCached := prompt - cached
	if nonCached < 0 {
		nonCached = 0
	}
	cost := float64(nonCached)*input/1_000_000 + float64(cached)*cacheRead/1_000_000 + float64(completion)*output/1_000_000
	return &cost
}
