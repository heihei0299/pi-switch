package server

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/heihei0299/pi-switch/internal/config"
)

// DORMANT: the retry/failover engine in this file is not wired into the request
// path. The proxy takes a single candidate and streams it straight through
// (`proxy_handlers.go`), `PUT /api/proxy/failover` answers 410, and every
// scheduling symbol below — `expandAttempts`, `admitForRound`, `waitForRound`,
// `classifyUpstreamError`, `classifyTransportError`, `coolForAttempt` and the
// cooldown helpers — has no caller outside the tests in this package. What is
// still live are the validators (`validateRetryFields`, `validateSettingsRetry`,
// called from the profile and settings handlers), which keep rejecting
// malformed retry knobs.
//
// The engine is retained on purpose rather than deleted:
// `.scratch/remove-failover-chain/spec.md` D1 keeps the primitives plus the
// `circuitBreaker` placeholder so a future per-conversation breaker can reuse
// them. Read it as a policy library with tests, not as the live strategy — do
// not expect a behavior change here to affect production, and do not delete it
// without checking that spec first.
//
// Retry scheduling modeled on CLIProxyAPI's credential retry rounds, adapted
// to pi-switch's Supplier/Channel model (one attempt = one profile channel,
// narrowed via narrowToChannel). Pure policy: no I/O here, so it stays unit-testable.
//
// Semantics:
//   - Round 0 is the initial round; round r only admits credentials whose
//     effective request-retry is at least r. Explicit 0 admits round 0 only,
//     omitted/negative inherits (channel -> profile -> global -> default 3).
//   - request-scoped-errors classify an (status, body) pair into an action.
//     Profile rules (when non-empty) override global rules; otherwise the
//     status defaults apply (403/408/429/5xx continue-and-cooldown, else stop).
//     Transport (network-level) failures carry no status and always
//     continue-and-cooldown; custom rules never match them.
//   - Cooldown is opt-out via disableCooling (channel -> profile -> global).
//     Transient (408/5xx/transport) cooldown defaults to legacy 60s.

const (
	defaultRequestRetry         = 3
	defaultMaxRetryIntervalSecs = 30
	legacyTransientCooldown     = 60 * time.Second
	maxRequestRetry             = 10
	streamErrorBodyLimit        = 64 << 10
)

type retryAction string

const (
	actionStop                retryAction = "stop"
	actionStopAndCooldown     retryAction = "stop-and-cooldown"
	actionContinue            retryAction = "continue"
	actionContinueAndCooldown retryAction = "continue-and-cooldown"
)

func (a retryAction) shouldStop() bool {
	return a == actionStop || a == actionStopAndCooldown
}

func (a retryAction) shouldCooldown() bool {
	return a == actionStopAndCooldown || a == actionContinueAndCooldown
}

func validRetryAction(s string) bool {
	switch retryAction(s) {
	case actionStop, actionStopAndCooldown, actionContinue, actionContinueAndCooldown:
		return true
	default:
		return false
	}
}

// retryNow and retrySleep are injectable for deterministic tests (no
// wall-clock sleeps in unit tests).
var (
	retryNow   = time.Now
	retrySleep = time.Sleep
)

var (
	cooldownMu    sync.Mutex
	cooldownUntil = map[string]time.Time{}
)

func cooldownKey(profile, baseURL string) string { return profile + "\x00" + baseURL }

func isCooling(key string) bool {
	cooldownMu.Lock()
	defer cooldownMu.Unlock()
	until, ok := cooldownUntil[key]
	if !ok {
		return false
	}
	if !retryNow().Before(until) {
		delete(cooldownUntil, key)
		return false
	}
	return true
}

func markCooling(key string, d time.Duration) {
	if d <= 0 {
		return
	}
	cooldownMu.Lock()
	defer cooldownMu.Unlock()
	cooldownUntil[key] = retryNow().Add(d)
}

// resetRetryStateForTest clears cooldown state between tests.
func resetRetryStateForTest() {
	cooldownMu.Lock()
	defer cooldownMu.Unlock()
	cooldownUntil = map[string]time.Time{}
}

// primaryChannel resolves the channel an attempt uses (mirrors
// PrimaryBaseURL/PrimaryAPIKey: upstreams[0] wins when set).
func primaryChannel(prof *config.ProviderProfile) *config.Upstream {
	if prof == nil {
		return nil
	}
	if len(prof.Upstreams) > 0 {
		u := prof.Upstreams[0]
		return &u
	}
	return nil
}

// narrowToChannel returns a copy of prof limited to its upsIdx-th resolved
// channel. Downstream policy helpers only understand the primary channel,
// so narrowing lets one attempt carry its own channel-level overrides
// (requestRetry, disableCooling) plus its baseUrl/apiKey/headers.
// Out-of-range idx returns prof unchanged; base resolution below then
// reports missing baseUrl as before.
func narrowToChannel(prof config.ProviderProfile, upsIdx int) config.ProviderProfile {
	ups := prof.ResolvedUpstreams()
	if upsIdx < 0 || upsIdx >= len(ups) {
		return prof
	}
	ch := ups[upsIdx]
	if ch.API != "" {
		prof.API = ch.API
	}
	if ch.ResponsesMode != "" {
		prof.ResponsesMode = ch.ResponsesMode
	}
	prof.Upstreams = []config.Upstream{ch}
	return prof
}

func channelWeight(u config.Upstream) uint32 {
	if u.Weight == nil {
		return 1
	}
	return *u.Weight
}

// orderedChannels lists ResolvedUpstreams indices heaviest-first (nil weight
// counts 1, ties keep config order). An explicit weight of 0 excludes the
// channel, mirroring CLIProxyAPI weighted routing.
func orderedChannels(prof *config.ProviderProfile) []int {
	if prof == nil {
		return nil
	}
	ups := prof.ResolvedUpstreams()
	var out []int
	for i, u := range ups {
		if u.Weight != nil && *u.Weight == 0 {
			continue
		}
		out = append(out, i)
	}
	sort.SliceStable(out, func(a, b int) bool {
		return channelWeight(ups[out[a]]) > channelWeight(ups[out[b]])
	})
	return out
}

// effectiveRequestRetry resolves channel -> profile -> global -> default.
func effectiveRequestRetry(cfg *config.PiSwitchConfig, prof *config.ProviderProfile) int {
	if prof != nil {
		if ch := primaryChannel(prof); ch != nil && ch.RequestRetry != nil && *ch.RequestRetry >= 0 {
			return min(*ch.RequestRetry, maxRequestRetry)
		}
		if prof.RequestRetry != nil && *prof.RequestRetry >= 0 {
			return min(*prof.RequestRetry, maxRequestRetry)
		}
	}
	if cfg != nil && cfg.Settings.Proxy.RequestRetry != nil && *cfg.Settings.Proxy.RequestRetry >= 0 {
		return min(*cfg.Settings.Proxy.RequestRetry, maxRequestRetry)
	}
	return defaultRequestRetry
}

// retryCoolingDisabled resolves channel -> profile -> global.
func retryCoolingDisabled(cfg *config.PiSwitchConfig, prof *config.ProviderProfile) bool {
	if prof != nil {
		if ch := primaryChannel(prof); ch != nil && ch.DisableCooling != nil {
			return *ch.DisableCooling
		}
		if prof.DisableCooling != nil {
			return *prof.DisableCooling
		}
	}
	if cfg != nil && cfg.Settings.Proxy.DisableCooling != nil {
		return *cfg.Settings.Proxy.DisableCooling
	}
	return false
}

// transientCooldown returns the cooldown for 408/5xx/transport failures.
// ok=false means cooldown disabled for this class.
func transientCooldown(cfg *config.PiSwitchConfig, prof *config.ProviderProfile) (d time.Duration, ok bool) {
	if retryCoolingDisabled(cfg, prof) {
		return 0, false
	}
	if cfg != nil {
		if v := cfg.Settings.Proxy.TransientErrorCooldownSeconds; v != nil && *v < 0 {
			return 0, false
		} else if v != nil && *v > 0 {
			return time.Duration(*v) * time.Second, true
		}
	}
	return legacyTransientCooldown, true
}

// maxRetryWait caps the inter-round cooldown wait (<=0 = never wait).
func maxRetryWait(cfg *config.PiSwitchConfig) time.Duration {
	if cfg != nil {
		if v := cfg.Settings.Proxy.MaxRetryInterval; v != nil {
			if *v <= 0 {
				return 0
			}
			return time.Duration(*v) * time.Second
		}
	}
	return defaultMaxRetryIntervalSecs * time.Second
}

func activeScopedRules(prof *config.ProviderProfile, cfg *config.PiSwitchConfig) []config.RequestScopedError {
	if prof != nil && len(prof.RequestScopedErrors) > 0 {
		return prof.RequestScopedErrors
	}
	if cfg != nil {
		return cfg.Settings.Proxy.RequestScopedErrors
	}
	return nil
}

func ruleMatches(rule config.RequestScopedError, body []byte) bool {
	if len(rule.Match) == 0 {
		return true
	}
	s := string(body)
	for _, m := range rule.Match {
		if m != "" && strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// classifyUpstreamError maps an upstream HTTP failure to a retry action.
// Custom rules match on status first, then optional body substrings.
func classifyUpstreamError(status int, body []byte, prof *config.ProviderProfile, cfg *config.PiSwitchConfig) retryAction {
	for _, rule := range activeScopedRules(prof, cfg) {
		if rule.Status == status && validRetryAction(rule.Action) && ruleMatches(rule, body) {
			return retryAction(rule.Action)
		}
	}
	switch status {
	case 403, 408, 429, 500, 502, 503, 504:
		return actionContinueAndCooldown
	default:
		return actionStop
	}
}

// coolForAttempt records a cooldown for key when the action requires it.
// The duration follows transientCooldown (cooling disabled degrades
// stop-and-cooldown to stop and continue-and-cooldown to continue).
func coolForAttempt(key string, act retryAction, cfg *config.PiSwitchConfig, prof *config.ProviderProfile) {
	if !act.shouldCooldown() {
		return
	}
	if d, ok := transientCooldown(cfg, prof); ok {
		markCooling(key, d)
	}
}

// classifyTransportError maps a network-level failure (no HTTP status).
func classifyTransportError(prof *config.ProviderProfile, cfg *config.PiSwitchConfig) retryAction {
	return actionContinueAndCooldown
}

// channelRef identifies one attemptable channel: a profile plus its index
// in that profile's orderedChannels.
type channelRef struct {
	name   string
	upsIdx int
}

// attempt plans one channelRef pass in round-major order.
type attempt struct {
	ref   channelRef
	round int
}

// profForAttempt resolves the candidate profile narrowed to the attempt
// channel; ok=false when the profile vanished from config.
func profForAttempt(cfg *config.PiSwitchConfig, att attempt) (prof config.ProviderProfile, ok bool) {
	if cfg == nil {
		return config.ProviderProfile{}, false
	}
	p, ok := cfg.Profiles[att.ref.name]
	if !ok {
		return config.ProviderProfile{}, false
	}
	return narrowToChannel(p, att.ref.upsIdx), true
}

// expandAttempts lists round-major attempts: round 0 admits every channel
// (profiles in candidate order, channels heaviest-first), round r only
// those with effective retry >= r. The per-round budget is enforced
// dynamically by admitForRound so cooling-skipped profiles never starve
// healthy ones behind it.
func expandAttempts(candidates []string, cfg *config.PiSwitchConfig) []attempt {
	eff := map[channelRef]int{}
	var order []channelRef
	maxR := 0
	if cfg != nil {
		for _, n := range candidates {
			prof, ok := cfg.Profiles[n]
			if !ok {
				continue
			}
			for _, u := range orderedChannels(&prof) {
				ref := channelRef{name: n, upsIdx: u}
				np := narrowToChannel(prof, u)
				e := effectiveRequestRetry(cfg, &np)
				eff[ref] = e
				if e > maxR {
					maxR = e
				}
				order = append(order, ref)
			}
		}
	}
	var out []attempt
	for r := 0; r <= maxR; r++ {
		for _, ref := range order {
			if eff[ref] < r {
				continue
			}
			out = append(out, attempt{ref: ref, round: r})
		}
	}
	return out
}

// admitForRound enforces the per-round distinct-profile budget (<=0 = all).
// Only fired attempts call it, so cooling-skipped profiles never consume
// the budget of healthy ones behind them.
func admitForRound(tried map[int]map[string]bool, att attempt, maxCreds int) bool {
	if maxCreds <= 0 {
		return true
	}
	m := tried[att.round]
	if m == nil {
		m = map[string]bool{}
		tried[att.round] = m
	}
	if m[att.ref.name] {
		return true
	}
	if len(m) >= maxCreds {
		return false
	}
	m[att.ref.name] = true
	return true
}

// waitForRound sleeps (bounded by maxWait) until at least one cooling key
// expires. It returns false when no wait was needed or allowed, in which
// case the caller must not spin further rounds with zero eligible attempts.
func waitForRound(keys []string, maxWait time.Duration) bool {
	if maxWait <= 0 || len(keys) == 0 {
		return false
	}
	now := retryNow()
	earliest := time.Time{}
	cooldownMu.Lock()
	for _, k := range keys {
		if until, ok := cooldownUntil[k]; ok && now.Before(until) {
			if earliest.IsZero() || until.Before(earliest) {
				earliest = until
			}
		}
	}
	cooldownMu.Unlock()
	if earliest.IsZero() {
		return false
	}
	wait := earliest.Sub(now)
	if wait <= 0 {
		return false
	}
	if wait > maxWait {
		return false
	}
	retrySleep(wait)
	return true
}

// candidateCooldownKeys lists cooldown keys for every channel of the
// candidates present in cfg.
func candidateCooldownKeys(candidates []string, cfg *config.PiSwitchConfig) []string {
	var keys []string
	if cfg == nil {
		return nil
	}
	for _, n := range candidates {
		if p, ok := cfg.Profiles[n]; ok {
			ups := p.ResolvedUpstreams()
			for _, u := range orderedChannels(&p) {
				keys = append(keys, cooldownKey(n, ups[u].BaseURL))
			}
		}
	}
	return keys
}

// coolingHint diagnoses an all-cooling failure; it returns "" when no key
// is cooling, letting the caller keep the underlying config error instead.
func coolingHint(keys []string) string {
	now := retryNow()
	var earliest time.Time
	cooldownMu.Lock()
	for _, k := range keys {
		if until, ok := cooldownUntil[k]; ok && now.Before(until) {
			if earliest.IsZero() || until.Before(earliest) {
				earliest = until
			}
		}
	}
	cooldownMu.Unlock()
	if earliest.IsZero() {
		return ""
	}
	return fmt.Sprintf(" (all candidates cooling, earliest retry in %s)", earliest.Sub(now).Round(time.Second))
}

// validateRetryFields checks the retry-related knobs of one profile for the
// management write paths (400 on violation).
func validateRetryFields(p config.ProviderProfile) error {
	if err := checkRequestRetry(p.RequestRetry, "requestRetry"); err != nil {
		return err
	}
	for _, u := range p.Upstreams {
		if err := checkRequestRetry(u.RequestRetry, "upstreams.requestRetry"); err != nil {
			return err
		}
	}
	if err := validateScopedRules(p.RequestScopedErrors, "requestScopedErrors"); err != nil {
		return err
	}
	return nil
}

func validateScopedRules(rules []config.RequestScopedError, what string) error {
	for i, rule := range rules {
		if rule.Status < 100 || rule.Status > 599 {
			return fmt.Errorf("%s[%d].status must be 100-599, got %d", what, i, rule.Status)
		}
		if !validRetryAction(rule.Action) {
			return fmt.Errorf("%s[%d].action must be stop|stop-and-cooldown|continue|continue-and-cooldown, got %q", what, i, rule.Action)
		}
	}
	return nil
}

func checkRequestRetry(v *int, field string) error {
	if v == nil || *v < 0 {
		return nil // nil/negative inherits
	}
	if *v > maxRequestRetry {
		return fmt.Errorf("%s must be 0-%d, got %d", field, maxRequestRetry, *v)
	}
	return nil
}

// validateSettingsRetry checks the global retry knobs for PUT /api/settings.
func validateSettingsRetry(s config.Settings) error {
	px := s.Proxy
	if err := checkRequestRetry(px.RequestRetry, "proxy.requestRetry"); err != nil {
		return err
	}
	if px.MaxRetryCredentials < 0 {
		return fmt.Errorf("proxy.maxRetryCredentials must be >= 0, got %d", px.MaxRetryCredentials)
	}
	if px.MaxRetryInterval != nil && *px.MaxRetryInterval < 0 {
		// Negative means "never wait" only via explicit <=0 check at runtime;
		// accept any negative as never-wait (no validation error).
		return nil
	}
	if v := px.TransientErrorCooldownSeconds; v != nil && *v < -1 {
		return fmt.Errorf("proxy.transientErrorCooldownSeconds must be >= -1, got %d", *v)
	}
	if err := validateScopedRules(px.RequestScopedErrors, "proxy.requestScopedErrors"); err != nil {
		return err
	}
	return nil
}
