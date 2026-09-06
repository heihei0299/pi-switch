package proxy

import (
	"fmt"
	"math"
)

// ClampMaxTokens implements spec limit logic:
// est = ceil(jsonLen/3), available = contextWindow - est - max(8192, window/128), clamped = max(16, available) then min with maxTokens.
// If requested != nil and <= clamped, keep requested; if > clamped, rewrite to clamped; if nil, return clamped.
// Returns clamped value and a detail string for logging.
func ClampMaxTokens(contextWindow, maxTokens uint32, rawJSONLen int, requested *int) (int, string) {
	est := int(math.Ceil(float64(rawJSONLen) / 3.0))
	safety := 8192
	if int(contextWindow/128) > safety {
		safety = int(contextWindow / 128)
	}
	available := int(contextWindow) - est - safety
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
