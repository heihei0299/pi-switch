package inference

import (
	"encoding/json"
	"fmt"

	"github.com/heihei0299/pi-switch/internal/domain/usage"
)

type processedResponse struct {
	Body  []byte
	Usage *usage.UsageSummary
	Cost  *float64
}

func processNonStreamResponse(request PreparedRequest, raw []byte) (processedResponse, error) {
	body := raw
	var response map[string]interface{}
	if request.ProtocolPlan.NeedsConvert() {
		if err := json.Unmarshal(raw, &response); err != nil {
			return processedResponse{}, fmt.Errorf("upstream response is not valid JSON: %w", err)
		}
		converted, err := request.ProtocolPlan.TransformResponse(response, request.Model)
		if err != nil {
			return processedResponse{}, err
		}
		body, err = json.Marshal(converted)
		if err != nil {
			return processedResponse{}, fmt.Errorf("converted response is not serializable: %w", err)
		}
	} else {
		_ = json.Unmarshal(raw, &response)
	}
	usageSum := usage.ExtractUsage(response)
	return processedResponse{Body: body, Usage: usageSum, Cost: usage.CalcUsageCost(request.ModelEntry, usageSum)}, nil
}
