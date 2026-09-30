package proxy

import (
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/usage"
)

func TestCalcCost_WithRates(t *testing.T) {
	me := config.ModelEntry{
		ID:   "gpt-4o-mini",
		Cost: &config.ModelCost{Input: 0.15, Output: 0.6, CacheRead: 0.075},
	}
	// prompt 120, cached 10, completion 80
	// nonCached 110*0.15/1e6=0.0000165, cached 10*0.075/1e6=0.00000075, completion 80*0.6/1e6=0.000048 -> total 0.00006525
	cost := CalcCost(&me, 120, 80, 10)
	if cost == nil {
		t.Fatal("cost nil, want non-nil")
	}
	// approx
	want := 0.00006525
	diff := *cost - want
	if diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("cost = %v, want %v", *cost, want)
	}
}

func TestCalcUsageCostPricesCacheWritesSeparately(t *testing.T) {
	s := usage.ExtractUsage(map[string]interface{}{"usage": map[string]interface{}{
		"input_tokens": 100, "cache_creation_input_tokens": 200, "cache_read_input_tokens": 700, "output_tokens": 50,
	}})
	for _, writeRate := range []float64{3.75, 0} {
		entry := config.ModelEntry{Cost: &config.ModelCost{Input: 3, Output: 15, CacheRead: 0.3, CacheWrite: writeRate}}
		cost := CalcUsageCost(&entry, s)
		want := (100*3 + 200*writeRate + 700*0.3 + 50*15) / 1_000_000
		if cost == nil || *cost < want-1e-10 || *cost > want+1e-10 {
			t.Fatalf("write rate=%v, cost=%v, want=%v", writeRate, cost, want)
		}
	}
}

func TestMissingAnthropicCacheWriteDoesNotInventZeroCost(t *testing.T) {
	entry := config.ModelEntry{Cost: &config.ModelCost{Input: 3, Output: 15, CacheRead: 0.3, CacheWrite: 3.75}}
	s := usage.ExtractUsage(map[string]interface{}{"usage": map[string]interface{}{
		"input_tokens": 100, "cache_read_input_tokens": 0, "output_tokens": 50,
	}})
	if cost := CalcUsageCost(&entry, s); cost != nil {
		t.Fatalf("missing cache-write usage fabricated cost=%v", *cost)
	}
}

func TestCalcCost_NilCostYieldsNil(t *testing.T) {
	me := config.ModelEntry{ID: "no-cost"}
	cost := CalcCost(&me, 100, 100, 0)
	if cost != nil {
		t.Fatalf("cost = %v, want nil", *cost)
	}
}

func TestCalcCost_NegativeNonCachedClamped(t *testing.T) {
	me := config.ModelEntry{
		ID:   "test",
		Cost: &config.ModelCost{Input: 2.0, Output: 8.0, CacheRead: 1.0},
	}
	// prompt 5, cached 10 (more cached than prompt, edge)
	cost := CalcCost(&me, 5, 10, 10)
	if cost == nil {
		t.Fatal("cost nil")
	}
	// nonCached clamped to 0, so cost = 10*1/1e6 +10*8/1e6=90/1e6=0.00009
	want := 0.00009
	diff := *cost - want
	if diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("cost = %v, want %v", *cost, want)
	}
}

func TestCalcCost_NilEntryYieldsNil(t *testing.T) {
	if cost := CalcCost(nil, 100, 100, 0); cost != nil {
		t.Fatalf("cost = %v, want nil for nil entry", *cost)
	}
}
