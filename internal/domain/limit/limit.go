package limit

import "math"

func ClampMaxTokens(contextWindow uint32, maxTokens uint32, jsonLen int, requested *int) int {
	est := int(math.Ceil(float64(jsonLen) / 3.0))
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
	if requested != nil {
		if *requested > clamped {
			return clamped
		}
		return *requested
	}
	return clamped
}
