package conversation

import (
	"testing"
	"time"
)

func TestMatchSourcePrecedence(t *testing.T) {
	timeNow := time.Date(2026, 9, 10, 1, 0, 0, 0, time.UTC)
	candidates := []Candidate{{ID: "session", Model: "m", LastActiveAt: timeNow}}

	if got := Match(SourceOff, MatchInput{ExplicitID: "explicit"}, candidates); got.ID != UnlabeledID {
		t.Fatalf("off must win before explicit id: %+v", got)
	}
	if got := Match(SourceProxy, MatchInput{ExplicitID: "explicit", ExplicitName: "A"}, candidates); got.ID != "explicit" || got.Source != MatchSourceExplicit {
		t.Fatalf("explicit id must win for proxy source: %+v", got)
	}
	if got := Match(SourceProxy, MatchInput{}, candidates); got.ID != UnlabeledID {
		t.Fatalf("proxy without id must be unlabeled: %+v", got)
	}
}

func TestMatchNormalizesBareModelAndUsesPromptAsSecondaryScore(t *testing.T) {
	timeNow := time.Date(2026, 9, 10, 1, 0, 0, 0, time.UTC)
	p1, p2 := uint64(100), uint64(200)
	got := Match(SourceSessionScan, MatchInput{
		Model:        "supplier/channel/model-a",
		Timestamp:    timeNow,
		PromptTokens: &p2,
	}, []Candidate{
		{ID: "near", Name: "near", Model: "model-a", LastActiveAt: timeNow.Add(1 * time.Millisecond), PromptTokensHint: &p1},
		{ID: "prompt", Name: "prompt", Model: "model-a", LastActiveAt: timeNow.Add(2 * time.Millisecond), PromptTokensHint: &p2},
	})
	if got.ID != "near" {
		t.Fatalf("time must be primary over prompt hint: %+v", got)
	}
}

func TestMatchAmbiguousCandidatesNeverGuesses(t *testing.T) {
	timeNow := time.Date(2026, 9, 10, 1, 0, 0, 0, time.UTC)
	candidates := []Candidate{
		{ID: "b", Model: "m", LastActiveAt: timeNow},
		{ID: "a", Model: "m", LastActiveAt: timeNow},
	}
	for i := 0; i < 2; i++ {
		got := Match(SourceSessionScan, MatchInput{Model: "m", Timestamp: timeNow}, candidates)
		if got.ID != UnlabeledID {
			t.Fatalf("equal candidates must be unlabeled, got %+v", got)
		}
		candidates[0], candidates[1] = candidates[1], candidates[0]
	}
}

func TestMatchDoesNotUseCandidatesOutsideWindow(t *testing.T) {
	now := time.Date(2026, 9, 10, 1, 0, 0, 0, time.UTC)
	got := Match(SourceSessionScan, MatchInput{Model: "m", Timestamp: now}, []Candidate{{
		ID: "old", Model: "m", LastActiveAt: now.Add(2001 * time.Millisecond),
	}})
	if got.ID != UnlabeledID {
		t.Fatalf("candidate beyond the two second window must be unlabeled: %+v", got)
	}
}

func TestSanitizeDisplayName(t *testing.T) {
	if got := SanitizeDisplayName("hello%20world%0Aname"); got != "hello world name" {
		t.Fatalf("sanitized name=%q", got)
	}
	if got := SanitizeDisplayName("  raw\tname  "); got != "raw name" {
		t.Fatalf("control characters not normalized: %q", got)
	}
}
