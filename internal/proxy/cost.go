package proxy

import (
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/usage"
)

// CalcCost prices usage without separately reported cache writes:
// (prompt-cached)*input + cached*cacheRead + completion*output, divided by 1M.
// Returns nil when entry is nil or has no cost (unknown).
func CalcCost(entry *config.ModelEntry, prompt, completion, cached int) *float64 {
	return calcCost(entry, float64(prompt), float64(completion), float64(cached), 0)
}

func calcCost(entry *config.ModelEntry, prompt, completion, cached, cacheWrite float64) *float64 {
	if entry == nil || entry.Cost == nil {
		return nil
	}
	// Rates are per 1M tokens.
	input := entry.Cost.Input
	output := entry.Cost.Output
	cacheRead := entry.Cost.CacheRead
	// When rates are zero and not set, still treat as valid (cost may be 0); only nil cost yields unknown.
	nonCached := prompt - cached - cacheWrite
	if nonCached < 0 {
		nonCached = 0
	}
	cost := (nonCached*input + cached*cacheRead + cacheWrite*entry.Cost.CacheWrite + completion*output) / 1_000_000
	return &cost
}

// CalcUsageCost only prices requests whose required usage facts are known.
func CalcUsageCost(entry *config.ModelEntry, summary *usage.UsageSummary) *float64 {
	if entry == nil || entry.Cost == nil || summary == nil || !summary.PromptTokensKnown || !summary.CompletionTokensKnown {
		return nil
	}
	if summary.PromptTokens > 0 && !summary.CachedTokensKnown && entry.Cost.Input != entry.Cost.CacheRead {
		return nil
	}
	return calcCost(entry, float64(summary.PromptTokens), float64(summary.CompletionTokens), float64(summary.CachedTokens), float64(summary.CacheWriteTokens))
}
