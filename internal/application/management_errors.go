package application

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/heihei0299/pi-switch/internal/config"
)

type ManagementValidationError struct{ Message string }

func (e *ManagementValidationError) Error() string { return e.Message }

// Validation retains field diagnostics without echoing stored or draft secrets.
func providerValidationError(prof config.ProviderProfile, err error) error {
	if err == nil {
		return nil
	}
	return &ManagementValidationError{Message: redactProviderMessage(prof, err.Error())}
}

func redactProviderMessage(prof config.ProviderProfile, message string) string {
	redact := func(value string) {
		if value != "" {
			message = strings.ReplaceAll(message, value, "[credential hidden]")
			quoted := strconv.Quote(value)
			if escaped := quoted[1 : len(quoted)-1]; escaped != value {
				message = strings.ReplaceAll(message, escaped, "[credential hidden]")
			}
		}
	}
	credentials := func(endpoint, key string, headers map[string]string) {
		redact(endpoint)
		redact(key)
		for _, value := range headers {
			redact(value)
		}
		if parsed, err := url.Parse(endpoint); err == nil {
			if parsed.User != nil {
				redact(parsed.User.Username())
				password, _ := parsed.User.Password()
				redact(password)
			}
			for _, values := range parsed.Query() {
				for _, value := range values {
					redact(value)
				}
			}
		}
	}
	credentials(prof.BaseURL, prof.APIKey, prof.Headers)
	for _, u := range prof.Upstreams {
		credentials(u.BaseURL, u.APIKey, u.Headers)
	}
	return message
}
