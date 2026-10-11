package application

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/profile"
)

// FetchUpstreamUsage is shared by Native and HTTP; untrusted response bodies
// and credential-bearing request URLs never become an error message.
func FetchUpstreamUsage(baseURL, apiKey, userAgent string) (map[string]interface{}, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseUrl is empty")
	}
	u := strings.TrimRight(baseURL, "/") + "/usage"
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, errors.New("usage request could not be created")
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("usage endpoint is unreachable")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.New("usage response could not be read")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("usage endpoint returned %d", resp.StatusCode)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, errors.New("usage endpoint returned invalid JSON")
	}
	usage, ok := decoded["usage"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("usage endpoint response has no usage object")
	}
	return usage, nil
}

type ProviderUsageWindow struct {
	Name     string
	Percent  *float64
	Status   string
	ResetsAt string
}

func LoadProviderUsage(path, name, expected string) ([]ProviderUsageWindow, error) {
	cfg, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		return nil, err
	}
	prof, ok := cfg.Profiles[name]
	if !ok {
		return nil, profile.ErrProfileNotFound
	}
	if Revision(prof) != expected {
		return nil, ErrManagementConflict
	}
	hasModels := false
	for _, u := range prof.Upstreams {
		hasModels = hasModels || len(u.Models) > 0
	}
	if prof.PrimaryBaseURL() == "" || !hasModels {
		return nil, &ManagementValidationError{Message: "Usage unavailable: no upstream endpoint or channel models. Unknown usage is not zero."}
	}
	usage, err := FetchUpstreamUsage(prof.PrimaryBaseURL(), prof.PrimaryAPIKey(), config.ResolveUserAgentValues(prof.UserAgent, cfg.Settings.Proxy.UserAgent))
	if err != nil {
		return nil, &ManagementValidationError{Message: err.Error()}
	}
	windows := make([]ProviderUsageWindow, 0, 3)
	for _, name := range []string{"rolling", "weekly", "monthly"} {
		window := ProviderUsageWindow{Name: name, Status: "unknown"}
		item, _ := usage[name].(map[string]interface{})
		if percent, ok := item["percent"].(float64); ok && !math.IsNaN(percent) && !math.IsInf(percent, 0) && percent >= 0 {
			window.Percent = &percent
		}
		if status, _ := item["status"].(string); status == "ok" || status == "warning" || status == "exhausted" || status == "blocked" || status == "disabled" {
			window.Status = status
		}
		if reset, _ := item["resetsAt"].(string); reset != "" {
			if date, err := time.Parse(time.RFC3339Nano, reset); err == nil {
				window.ResetsAt = date.Format(time.RFC3339)
			}
		}
		windows = append(windows, window)
	}
	return windows, nil
}
