package proxy

import (
	"fmt"
	"math"
)

// ClampMaxTokens implements spec limit logic:
// est = ceil(jsonLen/4), available = contextWindow - est - 4096, clamped = max(16, available) then min with maxTokens.
// If requested != nil and <= clamped, keep requested; if > clamped, rewrite to clamped; if nil, return clamped.
// Returns clamped value and a detail string for logging.
func ClampMaxTokens(contextWindow, maxTokens uint32, rawJSONLen int, requested *int) (int, string) {
	est := int(math.Ceil(float64(rawJSONLen) / 4.0))
	available := int(contextWindow) - est - 4096
	if available < 16 {
		available = 16
	}
	clamped := available
	if int(maxTokens) < clamped {
		clamped = int(maxTokens)
	}
	detail := fmt.Sprintf("est=%d window=%d avail=%d maxTokens=%d -> clamped=%d", est, contextWindow, available, maxTokens, clamped)
	if requested != nil {
		if *requested > clamped {
			detail += fmt.Sprintf(" (requested %d exceeds, rewritten)", *requested)
			return clamped, detail
		}
		detail += fmt.Sprintf(" (requested %d within)", *requested)
		return *requested, detail
	}
	return clamped, detail
}
