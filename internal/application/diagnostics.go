package application

import (
	"fmt"
	"sort"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/profile"
	"github.com/heihei0299/pi-switch/internal/protocol"
)

type DoctorCheck struct {
	OK      bool   `json:"ok"`
	Message string `json:"msg"`
}

type ValidationIssue struct {
	Level   string `json:"level"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

func DiagnoseConfig(path string) ([]DoctorCheck, []ValidationIssue) {
	cfg, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		return []DoctorCheck{{OK: false, Message: "config unreadable: " + err.Error()}}, []ValidationIssue{{Level: "error", Path: "config", Message: err.Error()}}
	}
	checks := []DoctorCheck{
		{OK: true, Message: "config JSON is valid"},
		{OK: len(cfg.Profiles) > 0, Message: fmt.Sprintf("%d profile(s) configured", len(cfg.Profiles))},
	}
	issues := make([]ValidationIssue, 0)
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		prof := cfg.Profiles[name]
		firstIssue := len(issues)
		prefix := "profiles." + name + "."
		if prof.API == "" {
			issues = append(issues, ValidationIssue{Level: "error", Path: prefix + "api", Message: "api required"})
		} else if !protocol.IsKnown(prof.API) {
			issues = append(issues, ValidationIssue{Level: "error", Path: prefix + "api", Message: "unsupported api " + prof.API})
		} else if len(prof.Upstreams) == 0 {
			if err := config.ValidateEffectiveFlatAPI(prof); err != nil {
				issues = append(issues, ValidationIssue{Level: "error", Path: prefix + "api", Message: err.Error()})
			}
		}
		if prof.BaseURL == "" && len(prof.Upstreams) == 0 {
			issues = append(issues, ValidationIssue{Level: "error", Path: prefix + "baseUrl", Message: "baseUrl required"})
		}
		hasModels := false
		for _, upstream := range prof.Upstreams {
			hasModels = hasModels || len(upstream.Models) > 0
		}
		if !hasModels {
			issues = append(issues, ValidationIssue{Level: "warning", Path: prefix + "upstreams", Message: "no models"})
		}
		if prof.ModelsDevProvider != nil && *prof.ModelsDevProvider != "" {
			known := map[string]bool{"openai": true, "anthropic": true, "google": true, "deepseek": true, "xai": true, "moonshot": true, "qwen": true, "cohere": true, "mistral": true, "azure": true, "custom": true}
			if !known[*prof.ModelsDevProvider] {
				issues = append(issues, ValidationIssue{Level: "warning", Path: prefix + "modelsDevProvider", Message: "modelsDevProvider unknown: " + *prof.ModelsDevProvider})
			}
		}
		if err := profile.ValidateResponsesMode(prof); err != nil {
			issues = append(issues, ValidationIssue{Level: "error", Path: prefix + "responsesMode", Message: err.Error()})
		}
		if err := config.ValidateProviderRetry(prof); err != nil {
			issues = append(issues, ValidationIssue{Level: "error", Path: prefix + "retry", Message: err.Error()})
		}
		for _, issue := range profile.ProfileIssues(prof) {
			issues = append(issues, ValidationIssue{Level: "error", Path: prefix + issue.Field, Message: issue.Message})
		}
		for i := firstIssue; i < len(issues); i++ {
			issues[i].Message = redactProviderMessage(prof, issues[i].Message)
		}
	}
	if err := config.ValidateSettingsRetry(cfg.Settings); err != nil {
		issues = append(issues, ValidationIssue{Level: "error", Path: "settings.proxy.retry", Message: "Invalid proxy retry settings; check retry counts, intervals and requestScopedErrors status/match/action rules."})
	}
	return checks, issues
}
