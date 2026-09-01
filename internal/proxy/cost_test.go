package proxy

import (
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

func TestCalcCost_WithRates(t *testing.T) {
	me := config.ModelEntry{
		ID: "gpt-4o-mini",
		Cost: &config.ModelCost{Input: 0.15, Output: 0.6, CacheRead: 0.075},
	}
	// prompt 120, cached 10, completion 80
	// nonCached 110*0.15/1e6=0.0000165, cached 10*0.075/1e6=0.00000075, completion 80*0.6/1e6=0.000048 -> total 0.00006525
	cost := CalcCost(me, 120, 80, 10)
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

func TestCalcCost_NilCostYieldsNil(t *testing.T) {
	me := config.ModelEntry{ID: "no-cost"}
	cost := CalcCost(me, 100, 100, 0)
	if cost != nil {
		t.Fatalf("cost = %v, want nil", *cost)
	}
}

func TestCalcCost_NegativeNonCachedClamped(t *testing.T) {
	me := config.ModelEntry{
		ID: "test",
		Cost: &config.ModelCost{Input: 2.0, Output: 8.0, CacheRead: 1.0},
	}
	// prompt 5, cached 10 (more cached than prompt, edge)
	cost := CalcCost(me, 5, 10, 10)
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
