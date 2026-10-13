package usage

import domainusage "github.com/heihei0299/pi-switch/internal/domain/usage"

type UsageSummary = domainusage.UsageSummary
type SseUsageParser = domainusage.SseUsageParser

func ExtractUsage(v map[string]interface{}) *UsageSummary { return domainusage.ExtractUsage(v) }

func NewSseUsageParser() *SseUsageParser { return domainusage.NewSseUsageParser() }
