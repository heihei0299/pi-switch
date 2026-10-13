package inference

import (
	"encoding/json"
	"fmt"
	"math"

	configdomain "github.com/heihei0299/pi-switch/internal/domain/config"
	"github.com/heihei0299/pi-switch/internal/domain/limit"
	"github.com/heihei0299/pi-switch/internal/domain/protocol"
	"github.com/heihei0299/pi-switch/internal/domain/provider"
)

type PreparationError struct {
	Type    string
	Message string
}

func (e *PreparationError) Error() string { return e.Message }

type PreparedRequest struct {
	Supplier     string
	Model        string
	Profile      provider.Profile
	Upstream     provider.Upstream
	ModelEntry   *provider.ModelEntry
	Body         map[string]interface{}
	ProtocolPlan protocol.Plan
	sourceBody   map[string]interface{}
	rawLength    int
}

func PrepareRequest(cfg configdomain.PiSwitchConfig, requestedModel, incomingProtocol string, body map[string]interface{}, rawLength int, streaming bool) (PreparedRequest, error) {
	route := ResolveRoute(cfg, requestedModel)
	if route.Ambiguous {
		return PreparedRequest{}, &PreparationError{Type: "ambiguous", Message: fmt.Sprintf("Ambiguous model '%s': exposed in multiple channels", requestedModel)}
	}
	if len(route.Candidates) == 0 {
		return PreparedRequest{}, &PreparationError{Type: "no_route", Message: fmt.Sprintf("No upstream exposes model '%s'", requestedModel)}
	}
	supplier := route.Candidates[0]
	profile, ok := cfg.Profiles[supplier]
	if !ok {
		return PreparedRequest{}, &PreparationError{Type: "no_route", Message: fmt.Sprintf("No upstream exposes model '%s'", requestedModel)}
	}
	if route.Channel != "" {
		found := false
		for i, upstream := range profile.ResolvedUpstreams() {
			channelName := ""
			if upstream.Name != nil {
				channelName = *upstream.Name
			}
			if channelName == route.Channel {
				profile = profile.NarrowToChannel(i)
				found = true
				break
			}
		}
		if !found {
			return PreparedRequest{}, &PreparationError{Type: "no_route", Message: fmt.Sprintf("No upstream exposes model '%s'", requestedModel)}
		}
	}
	if route.Channel == "" {
		matched, matchCount := -1, 0
		for i, upstream := range profile.ResolvedUpstreams() {
			for _, exposed := range upstream.ExposedModels {
				if exposed == route.Model {
					if matched == -1 {
						matched = i
					}
					matchCount++
					break
				}
			}
		}
		if matchCount > 1 {
			message := fmt.Sprintf("Ambiguous model '%s' in supplier '%s': exposed in %d channels, use '%s/<channel>/%s'", requestedModel, supplier, matchCount, supplier, route.Model)
			if streaming {
				message = fmt.Sprintf("Ambiguous model '%s/%s' in supplier '%s': exposed in %d channels, use '%s/<channel>/%s'", supplier, route.Model, supplier, matchCount, supplier, route.Model)
			}
			return PreparedRequest{}, &PreparationError{Type: "ambiguous", Message: message}
		}
		if matched != -1 {
			profile = profile.NarrowToChannel(matched)
		}
	}
	upstream := profile.PrimaryUpstream()
	if upstream.BaseURL == "" {
		return PreparedRequest{}, &PreparationError{Type: "no_route", Message: "missing baseUrl"}
	}
	modelEntry := findModelEntry(profile, route.Model)
	sourceBody := cloneRequestBody(body)
	sourceBody["model"] = route.Model
	if streaming {
		sourceBody["stream"] = true
	}
	clampBody(sourceBody, modelEntry, rawLength)
	plan, err := protocol.PlanRequest(incomingProtocol, profile.API, profile.ResponsesMode)
	if err != nil {
		return PreparedRequest{}, &PreparationError{Type: "no_route", Message: fmt.Sprintf("profile %s: %s", supplier, err.Error())}
	}
	if streaming && !plan.Passthrough && plan.StreamConverter(route.Model) == nil {
		return PreparedRequest{}, &PreparationError{Type: "not_supported", Message: fmt.Sprintf("streaming conversion from %s to %s is not supported", plan.To, plan.From)}
	}
	requestBody, err := plan.TransformRequest(route.Model, sourceBody)
	if err != nil {
		return PreparedRequest{}, &PreparationError{Type: "no_route", Message: err.Error()}
	}
	if streaming {
		requestBody["stream"] = true
	}
	clampBody(requestBody, modelEntry, rawLength)
	return PreparedRequest{
		Supplier: supplier, Model: route.Model, Profile: profile, Upstream: upstream,
		ModelEntry: modelEntry, Body: requestBody, ProtocolPlan: plan,
		sourceBody: sourceBody, rawLength: rawLength,
	}, nil
}

func (p PreparedRequest) MinTokenRetryBody() (map[string]interface{}, error) {
	body := cloneRequestBody(p.sourceBody)
	for _, key := range []string{"max_tokens", "max_output_tokens", "max_completion_tokens"} {
		body[key] = float64(16)
	}
	body, err := p.ProtocolPlan.TransformRequest(p.Model, body)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"max_tokens", "max_output_tokens", "max_completion_tokens"} {
		if _, ok := body[key]; ok {
			body[key] = float64(16)
		}
	}
	if _, ok := body["max_output_tokens"]; !ok {
		body["max_output_tokens"] = float64(16)
	}
	clampBody(body, p.ModelEntry, p.rawLength)
	return body, nil
}

func cloneRequestBody(body map[string]interface{}) map[string]interface{} {
	encoded, _ := json.Marshal(body)
	var copied map[string]interface{}
	_ = json.Unmarshal(encoded, &copied)
	return copied
}

func findModelEntry(profile provider.Profile, model string) *provider.ModelEntry {
	for i := range profile.Upstreams {
		for j := range profile.Upstreams[i].Models {
			if profile.Upstreams[i].Models[j].ID == model {
				return &profile.Upstreams[i].Models[j]
			}
		}
	}
	return nil
}

func clampBody(body map[string]interface{}, model *provider.ModelEntry, rawLength int) {
	if model == nil || model.ContextWindow == 0 || model.MaxTokens == 0 {
		return
	}
	effectiveLength := estimateEffectiveLen(body, rawLength)
	for _, key := range []string{"max_tokens", "max_output_tokens", "max_completion_tokens"} {
		if value, ok := body[key]; ok {
			var requested int
			switch value := value.(type) {
			case float64:
				requested = int(value)
			case int:
				requested = value
			default:
				continue
			}
			original := requested
			clamped := limit.ClampMaxTokens(model.ContextWindow, model.MaxTokens, effectiveLength, &requested)
			if clamped != original {
				body[key] = float64(clamped)
			}
		}
	}
}

func estimateEffectiveLen(body map[string]interface{}, rawLength int) int {
	// Encrypted reasoning content is denser than ordinary text, so account for it at 2.5 rather than 3 chars/token.
	var encryptedLength int
	var walk func(interface{})
	walk = func(value interface{}) {
		switch value := value.(type) {
		case map[string]interface{}:
			for key, child := range value {
				if key == "encrypted_content" {
					if text, ok := child.(string); ok {
						encryptedLength += len(text)
					}
				}
				walk(child)
			}
		case []interface{}:
			for _, child := range value {
				walk(child)
			}
		}
	}
	walk(body)
	if encryptedLength > 0 {
		return rawLength + int(math.Ceil(float64(encryptedLength)*0.2))
	}
	return rawLength
}
