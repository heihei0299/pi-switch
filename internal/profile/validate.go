package profile

import (
	"fmt"
	"strings"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/protocol"
)

// ValidateProfile is the whole business-rule gate for one profile, in the order the
// surfaces report it: responsesMode compatibility, then shape, then retry knobs. It
// is the single sequence behind CreateProfile (POST /api/profiles, `provider add`)
// and PUT /api/profiles/:name, which overwrites instead of creating and therefore
// cannot reuse CreateProfile itself.
func ValidateProfile(p config.ProviderProfile) error {
	if err := ValidateResponsesMode(p); err != nil {
		return err
	}
	if err := ValidateProviderProfile(p); err != nil {
		return err
	}
	return config.ValidateProviderRetry(p)
}

// ValidateResponsesMode rejects a responsesMode/api combination the proxy cannot
// execute. It delegates to the one protocol rule; an empty mode is "auto".
func ValidateResponsesMode(p config.ProviderProfile) error {
	return protocol.ValidateResponsesMode(p.API, p.ResponsesMode)
}

// ValidateProviderProfile checks the profile shape: base URLs, channel names and
// per-channel pool/exposed consistency.
func ValidateProviderProfile(p config.ProviderProfile) error {
	if p.BaseURL != "" && !strings.HasPrefix(p.BaseURL, "http://") && !strings.HasPrefix(p.BaseURL, "https://") {
		return fmt.Errorf("baseUrl must start with http:// or https://")
	}
	seenChannel := map[string]bool{}
	for _, u := range p.Upstreams {
		if u.BaseURL != "" && !strings.HasPrefix(u.BaseURL, "http://") && !strings.HasPrefix(u.BaseURL, "https://") {
			return fmt.Errorf("upstreams baseUrl must start with http:// or https://")
		}
		name := ""
		if u.Name != nil {
			name = *u.Name
		}
		if !isValidChannelName(name) {
			return fmt.Errorf("upstreams name %q invalid: required, 1-32 chars of [A-Za-z0-9-_]", name)
		}
		if seenChannel[name] {
			return fmt.Errorf("duplicate upstream name %q", name)
		}
		seenChannel[name] = true
		if err := config.ValidateUpstreamAPI(u, p); err != nil {
			return err
		}
		// 分区校验：池内 id 去重；暴露 id 必须归属本渠道池。
		poolSeen := map[string]bool{}
		for _, m := range u.Models {
			if strings.TrimSpace(m.ID) == "" {
				return fmt.Errorf("upstreams[%q] model id must not be empty", name)
			}
			if poolSeen[m.ID] {
				return fmt.Errorf("upstreams[%q] duplicate model id %q", name, m.ID)
			}
			poolSeen[m.ID] = true
		}
		for _, eid := range u.ExposedModels {
			if !poolSeen[eid] {
				return fmt.Errorf("upstreams[%q] exposedModels references unknown model %q", name, eid)
			}
		}
	}
	return nil
}

// isValidChannelName enforces the channel primary-key rule: required,
// 1-32 chars, letters/digits/'-'/"_" only (stable gateway id segment).
func isValidChannelName(name string) bool {
	if len(name) == 0 || len(name) > 32 {
		return false
	}
	for _, c := range name {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' {
			continue
		}
		return false
	}
	return true
}
