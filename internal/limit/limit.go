package limit

import "math"

func ClampMaxTokens(contextWindow uint32, maxTokens uint32, jsonLen int, requested *int) int {
	est := int(math.Ceil(float64(jsonLen) / 4.0))
	available := int(contextWindow) - est - 4096
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
