package config

import "fmt"

// MaxRequestRetry bounds the configurable retry rounds (additional rounds after
// round 0). It is the single source for both the proxy runtime clamp and the
// management write validation.
const MaxRequestRetry = 10

// RetryAction classifies an upstream failure for the proxy retry loop.
type RetryAction string

const (
	RetryStop                RetryAction = "stop"
	RetryStopAndCooldown     RetryAction = "stop-and-cooldown"
	RetryContinue            RetryAction = "continue"
	RetryContinueAndCooldown RetryAction = "continue-and-cooldown"
)

// ValidRetryAction reports whether s is one of the four accepted actions.
func ValidRetryAction(s string) bool {
	switch RetryAction(s) {
	case RetryStop, RetryStopAndCooldown, RetryContinue, RetryContinueAndCooldown:
		return true
	default:
		return false
	}
}

// CheckRequestRetry validates one retry-round knob. Nil and negative inherit, so
// only an explicit value above MaxRequestRetry is rejected.
func CheckRequestRetry(v *int, field string) error {
	if v == nil || *v < 0 {
		return nil // nil/negative inherits
	}
	if *v > MaxRequestRetry {
		return fmt.Errorf("%s must be 0-%d, got %d", field, MaxRequestRetry, *v)
	}
	return nil
}

// ValidateScopedRules checks the status/action pairs of an error-rule list.
func ValidateScopedRules(rules []RequestScopedError, what string) error {
	for i, rule := range rules {
		if rule.Status < 100 || rule.Status > 599 {
			return fmt.Errorf("%s[%d].status must be 100-599, got %d", what, i, rule.Status)
		}
		if !ValidRetryAction(rule.Action) {
			return fmt.Errorf("%s[%d].action must be stop|stop-and-cooldown|continue|continue-and-cooldown, got %q", what, i, rule.Action)
		}
	}
	return nil
}

// ValidateProviderRetry checks the retry-related knobs of one profile for the
// management write paths (400 on violation).
func ValidateProviderRetry(p ProviderProfile) error {
	if err := CheckRequestRetry(p.RequestRetry, "requestRetry"); err != nil {
		return err
	}
	for _, u := range p.Upstreams {
		if err := CheckRequestRetry(u.RequestRetry, "upstreams.requestRetry"); err != nil {
			return err
		}
	}
	return ValidateScopedRules(p.RequestScopedErrors, "requestScopedErrors")
}

// ValidateSettingsRetry checks the global retry knobs for PUT /api/settings.
func ValidateSettingsRetry(s Settings) error {
	px := s.Proxy
	if err := CheckRequestRetry(px.RequestRetry, "proxy.requestRetry"); err != nil {
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
	return ValidateScopedRules(px.RequestScopedErrors, "proxy.requestScopedErrors")
}
