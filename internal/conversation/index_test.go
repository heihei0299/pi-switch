package conversation

import (
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func TestIndexedMatcherMatchesScanForMixedCandidates(t *testing.T) {
	base := time.Date(2026, 9, 10, 1, 0, 0, 0, time.UTC)
	rng := rand.New(rand.NewSource(17))
	candidates := make([]Candidate, 512)
	for i := range candidates {
		model := []string{"m", "supplier/channel/m", "other", "foo/"}[rng.Intn(4)]
		provider := []string{"", "supplier", "other"}[rng.Intn(3)]
		var prompt *uint64
		if rng.Intn(2) == 0 {
			value := uint64(rng.Intn(500))
			prompt = &value
		}
		candidates[i] = Candidate{
			ID: "session", Name: "candidate", Provider: provider, Model: model,
			LastActiveAt:     base.Add(time.Duration(rng.Intn(20_000)-10_000) * time.Millisecond),
			PromptTokensHint: prompt,
		}
	}
	candidates = append(candidates,
		Candidate{ID: "", Model: "m", LastActiveAt: base},
		Candidate{ID: "zero", Model: "m"},
	)
	matcher := NewMatcher(candidates)
	for _, source := range []Source{SourceOff, SourceProxy, SourceSessionScan} {
		for i := 0; i < 100; i++ {
			model := []string{"m", "supplier/channel/m", "other", "foo/", "missing"}[rng.Intn(5)]
			provider := []string{"", "supplier", "other"}[rng.Intn(3)]
			var prompt *uint64
			if rng.Intn(2) == 0 {
				value := uint64(rng.Intn(500))
				prompt = &value
			}
			input := MatchInput{
				Model: model, Provider: provider,
				Timestamp:    base.Add(time.Duration(rng.Intn(30_000)-15_000) * time.Millisecond),
				PromptTokens: prompt,
			}
			if i%7 == 0 {
				input.ExplicitID = "explicit"
			}
			want := Match(source, input, candidates)
			got := matcher.Match(source, input)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("source=%s input=%+v: indexed=%+v, scan=%+v", source, input, got, want)
			}
		}
	}
}

func TestIndexedMatcherIncludesWindowEdges(t *testing.T) {
	base := time.Date(2026, 9, 10, 1, 0, 0, 0, time.UTC)
	for _, offset := range []time.Duration{-2 * time.Second, 2 * time.Second} {
		candidate := Candidate{ID: "edge", Model: "m", LastActiveAt: base.Add(offset)}
		got := NewMatcher([]Candidate{candidate}).Match(SourceSessionScan, MatchInput{Model: "m", Timestamp: base})
		if got.ID != "edge" {
			t.Errorf("offset %s was excluded at the inclusive match-window edge: %+v", offset, got)
		}
	}
	outside := Candidate{ID: "outside", Model: "m", LastActiveAt: base.Add(2*time.Second + time.Nanosecond)}
	if got := NewMatcher([]Candidate{outside}).Match(SourceSessionScan, MatchInput{Model: "m", Timestamp: base}); got.ID != UnlabeledID {
		t.Fatalf("candidate outside the match window was included: %+v", got)
	}
}
