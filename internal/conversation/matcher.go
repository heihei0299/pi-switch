package conversation

import (
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const UnlabeledID = "unlabeled"

type Source string

const (
	SourceProxy       Source = "proxy"
	SourceSessionScan Source = "sessionScan"
	SourceOff         Source = "off"
)

const (
	MatchSourceExplicit                = "explicit"
	MatchSourceSessionExact            = "session-scan-exact"
	MatchSourceSessionHeuristic        = "session-scan-heuristic"
	MatchSourceUnlabeled               = "unlabeled"
	MatchConfidenceExact               = "exact"
	MatchConfidenceHeuristic           = "heuristic"
	MatchConfidenceNone                = "none"
	matchWindow                        = 2 * time.Second
	matchAmbiguityWindow               = 10 * time.Millisecond
	unknownPromptDistance       uint64 = 1 << 62
)

type MatchInput struct {
	ExplicitID   string
	ExplicitName string
	Provider     string
	Model        string
	Timestamp    time.Time
	PromptTokens *uint64
}

type Candidate struct {
	ID               string
	Name             string
	Provider         string
	Model            string
	LastActiveAt     time.Time
	PromptTokensHint *uint64
}

type MatchResult struct {
	ID         string
	Name       string
	Source     string
	Confidence string
}

type scoredCandidate struct {
	candidate   Candidate
	timeDiff    time.Duration
	promptDiff  uint64
	promptKnown bool
}

// Match applies the single conversation attribution policy used by both
// request-time logging and Stats-time projection. It never consults the clock;
// callers provide the request row timestamp and a stable session snapshot.
func Match(source Source, input MatchInput, candidates []Candidate) MatchResult {
	if source == SourceOff {
		return unlabeled()
	}
	if id := strings.TrimSpace(input.ExplicitID); id != "" && id != UnlabeledID {
		return MatchResult{
			ID:         id,
			Name:       strings.TrimSpace(input.ExplicitName),
			Source:     MatchSourceExplicit,
			Confidence: MatchConfidenceExact,
		}
	}
	if source != SourceSessionScan {
		return unlabeled()
	}
	if input.Timestamp.IsZero() || strings.TrimSpace(input.Model) == "" {
		return unlabeled()
	}

	requestModel := strings.TrimSpace(input.Model)
	requestBareModel := BareModel(requestModel)
	requestProvider := NormalizeProvider(input.Provider)
	scored := make([]scoredCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.ID) == "" || candidate.LastActiveAt.IsZero() {
			continue
		}
		candidateProvider := NormalizeProvider(candidate.Provider)
		if requestProvider != "" && candidateProvider != "" && requestProvider != candidateProvider {
			continue
		}
		candidateModel := strings.TrimSpace(candidate.Model)
		if candidateModel == "" || !modelsEqual(requestModel, requestBareModel, candidateModel) {
			continue
		}
		diff := input.Timestamp.Sub(candidate.LastActiveAt)
		if diff < 0 {
			diff = -diff
		}
		if diff > matchWindow {
			continue
		}
		promptDiff, promptKnown := promptDistance(input.PromptTokens, candidate.PromptTokensHint)
		scored = append(scored, scoredCandidate{
			candidate:   candidate,
			timeDiff:    diff,
			promptDiff:  promptDiff,
			promptKnown: promptKnown,
		})
	}
	if len(scored) == 0 {
		return unlabeled()
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].timeDiff != scored[j].timeDiff {
			return scored[i].timeDiff < scored[j].timeDiff
		}
		if scored[i].promptKnown != scored[j].promptKnown {
			return scored[i].promptKnown
		}
		if scored[i].promptDiff != scored[j].promptDiff {
			return scored[i].promptDiff < scored[j].promptDiff
		}
		return scored[i].candidate.ID < scored[j].candidate.ID
	})

	best := scored[0]
	if len(scored) > 1 && ambiguous(best, scored[1]) {
		return unlabeled()
	}
	matchSource := MatchSourceSessionHeuristic
	confidence := MatchConfidenceHeuristic
	if best.timeDiff == 0 && best.promptKnown {
		matchSource = MatchSourceSessionExact
		confidence = MatchConfidenceExact
	}
	return MatchResult{
		ID:         best.candidate.ID,
		Name:       best.candidate.Name,
		Source:     matchSource,
		Confidence: confidence,
	}
}

func ambiguous(best, second scoredCandidate) bool {
	if best.timeDiff == second.timeDiff && best.promptKnown == second.promptKnown && best.promptDiff == second.promptDiff {
		return true
	}
	if second.timeDiff-best.timeDiff > matchAmbiguityWindow {
		return false
	}
	// A tiny timestamp advantage is not enough to guess between two sessions
	// when prompt hints cannot distinguish them.
	return !best.promptKnown && !second.promptKnown
}

func promptDistance(a, b *uint64) (uint64, bool) {
	if a == nil || b == nil {
		return unknownPromptDistance, false
	}
	if *a > *b {
		return *a - *b, true
	}
	return *b - *a, true
}

func modelsEqual(request, requestBare, candidate string) bool {
	candidate = strings.TrimSpace(candidate)
	return request == candidate || requestBare != "" && requestBare == BareModel(candidate)
}

// NormalizeProvider trims the provider identity at the matcher boundary.
func NormalizeProvider(provider string) string {
	return strings.TrimSpace(provider)
}

// BareModel returns the model identifier without supplier/channel prefixes.
// Gateway routes use profile/channel/model while pi session JSONL commonly
// stores only the last model component.
func BareModel(model string) string {
	model = strings.Trim(strings.TrimSpace(model), "/")
	if idx := strings.LastIndexByte(model, '/'); idx >= 0 {
		return model[idx+1:]
	}
	return model
}

// SanitizeDisplayName decodes the wire representation once at the display
// boundary and removes control characters that could break log/UI rows.
func SanitizeDisplayName(raw string) string {
	if raw == "" {
		return ""
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil || !utf8.ValidString(decoded) {
		decoded = raw
	}
	decoded = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(decoded)
	return strings.TrimSpace(decoded)
}

func unlabeled() MatchResult {
	return MatchResult{ID: UnlabeledID, Source: MatchSourceUnlabeled, Confidence: MatchConfidenceNone}
}
