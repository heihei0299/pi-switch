package inference

import (
	"encoding/json"
	"fmt"

	"github.com/heihei0299/pi-switch/internal/domain/usage"
)

type NonStreamResult struct {
	Body  []byte
	Usage *usage.UsageSummary
	Cost  *float64
}

func ProcessNonStreamResponse(request PreparedRequest, raw []byte) (NonStreamResult, error) {
	body := raw
	var response map[string]interface{}
	if request.ProtocolPlan.NeedsConvert() {
		if err := json.Unmarshal(raw, &response); err != nil {
			return NonStreamResult{}, fmt.Errorf("upstream response is not valid JSON: %w", err)
		}
		converted, err := request.ProtocolPlan.TransformResponse(response, request.Model)
		if err != nil {
			return NonStreamResult{}, err
		}
		body, err = json.Marshal(converted)
		if err != nil {
			return NonStreamResult{}, fmt.Errorf("converted response is not serializable: %w", err)
		}
	} else {
		_ = json.Unmarshal(raw, &response)
	}
	usageSum := usage.ExtractUsage(response)
	return NonStreamResult{Body: body, Usage: usageSum, Cost: usage.CalcUsageCost(request.ModelEntry, usageSum)}, nil
}
