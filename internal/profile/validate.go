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

// Issue is one profile diagnostic plus the field it belongs to, so the advisory
// surface can point at a path while the write paths only need the first message.
// Message keeps the exact wording the CRUD doors print.
type Issue struct {
	Field   string
	Message string
}

func (i Issue) Error() string { return i.Message }

// ProfileIssues lists every shape/model/channel problem of one profile, in the order
// the write paths report them. It is the whole rule, not a summary: the tolerant
// config door lets these through, so /api/config/validate has to be able to name all
// of them instead of stopping at the first.
func ProfileIssues(p config.ProviderProfile) []Issue {
	var issues []Issue
	add := func(field, message string) { issues = append(issues, Issue{Field: field, Message: message}) }

	if p.BaseURL != "" && !strings.HasPrefix(p.BaseURL, "http://") && !strings.HasPrefix(p.BaseURL, "https://") {
		add("baseUrl", "baseUrl must start with http:// or https://")
	}
	seenChannel := map[string]bool{}
	for i, u := range p.Upstreams {
		at := func(suffix string) string { return fmt.Sprintf("upstreams[%d].%s", i, suffix) }
		if u.BaseURL != "" && !strings.HasPrefix(u.BaseURL, "http://") && !strings.HasPrefix(u.BaseURL, "https://") {
			add(at("baseUrl"), "upstreams baseUrl must start with http:// or https://")
		}
		name := ""
		if u.Name != nil {
			name = *u.Name
		}
		if !isValidChannelName(name) {
			add(at("name"), fmt.Sprintf("upstreams name %q invalid: required, 1-32 chars of [A-Za-z0-9-_]", name))
		} else if seenChannel[name] {
			add(at("name"), fmt.Sprintf("duplicate upstream name %q", name))
		}
		seenChannel[name] = true
		if err := config.ValidateUpstreamAPI(u, p); err != nil {
			add(at("api"), err.Error())
		}
		// 分区校验：池内 id 去重；暴露 id 必须归属本渠道池。
		poolSeen := map[string]bool{}
		for j, m := range u.Models {
			if strings.TrimSpace(m.ID) == "" {
				add(fmt.Sprintf("upstreams[%d].models[%d].id", i, j), fmt.Sprintf("upstreams[%q] model id must not be empty", name))
				continue
			}
			if poolSeen[m.ID] {
				add(fmt.Sprintf("upstreams[%d].models[%d].id", i, j), fmt.Sprintf("upstreams[%q] duplicate model id %q", name, m.ID))
				continue
			}
			poolSeen[m.ID] = true
		}
		for j, eid := range u.ExposedModels {
			if !poolSeen[eid] {
				add(fmt.Sprintf("upstreams[%d].exposedModels[%d]", i, j), fmt.Sprintf("upstreams[%q] exposedModels references unknown model %q", name, eid))
			}
		}
	}
	return issues
}

// ValidateProviderProfile checks the profile shape: base URLs, channel names and
// per-channel pool/exposed consistency. It reports the first issue so the doors that
// author one profile answer with a single message; ProfileIssues is the full list.
func ValidateProviderProfile(p config.ProviderProfile) error {
	if issues := ProfileIssues(p); len(issues) > 0 {
		return issues[0]
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
