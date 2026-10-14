package config

import domainconfig "github.com/heihei0299/pi-switch/internal/domain/config"

const MaxRequestRetry = domainconfig.MaxRequestRetry

type RetryAction = domainconfig.RetryAction

const (
	RetryStop                = domainconfig.RetryStop
	RetryStopAndCooldown     = domainconfig.RetryStopAndCooldown
	RetryContinue            = domainconfig.RetryContinue
	RetryContinueAndCooldown = domainconfig.RetryContinueAndCooldown
)

func ValidRetryAction(s string) bool { return domainconfig.ValidRetryAction(s) }

func CheckRequestRetry(v *int, field string) error { return domainconfig.CheckRequestRetry(v, field) }

func ValidateScopedRules(rules []RequestScopedError, what string) error {
	return domainconfig.ValidateScopedRules(rules, what)
}

func ValidateProviderRetry(p ProviderProfile) error { return domainconfig.ValidateProviderRetry(p) }

func ValidateSettingsRetry(s Settings) error { return domainconfig.ValidateSettingsRetry(s) }
