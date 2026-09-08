package server

import (
	"bytes"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/heihei0299/pi-switch/internal/config"
)

type OutboundRequestPlan struct {
	Upstream         config.Upstream
	Path             string
	ProfileHeaders   map[string]string
	IncomingHeaders  http.Header
	Body             []byte
	ContentType      string
	Accept           string
	ProfileUserAgent *string
	GlobalUserAgent  *string
	Timeout          time.Duration
}

type OutboundRequestMetadata struct {
	URL              string
	Method           string
	HasAuthorization bool
	HeaderNames      []string
}

type BuiltOutboundRequest struct {
	Request  *http.Request
	Client   *http.Client
	Metadata OutboundRequestMetadata
}

func BuildOutboundRequest(plan OutboundRequestPlan) (BuiltOutboundRequest, error) {
	method := http.MethodPost
	url := buildUpstreamURL(plan.Upstream.BaseURL, plan.Path)
	req, err := http.NewRequest(method, url, bytes.NewReader(plan.Body))
	if err != nil {
		return BuiltOutboundRequest{}, err
	}

	if plan.ContentType != "" {
		req.Header.Set("Content-Type", plan.ContentType)
	}
	if plan.Accept != "" {
		req.Header.Set("Accept", plan.Accept)
	}
	if plan.Upstream.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+plan.Upstream.APIKey)
	}
	for key, value := range mergeOutboundHeaders(plan.ProfileHeaders, plan.Upstream.Headers) {
		req.Header.Set(key, value)
	}
	req.Header.Set("User-Agent", resolveUserAgentValues(plan.ProfileUserAgent, plan.GlobalUserAgent))
	applyOpenCodeSessionAffinityHeader(plan.Upstream.BaseURL, plan.IncomingHeaders, req.Header)

	headerNames := make([]string, 0, len(req.Header))
	for name := range req.Header {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	return BuiltOutboundRequest{
		Request: req,
		Client:  &http.Client{Timeout: plan.Timeout},
		Metadata: OutboundRequestMetadata{
			URL:              url,
			Method:           method,
			HasAuthorization: plan.Upstream.APIKey != "",
			HeaderNames:      headerNames,
		},
	}, nil
}

func mergeOutboundHeaders(profile, channel map[string]string) map[string]string {
	merged := make(map[string]string, len(profile)+len(channel))
	for key, value := range profile {
		setOutboundHeader(merged, key, value)
	}
	for key, value := range channel {
		setOutboundHeader(merged, key, value)
	}
	return merged
}

func selectedOutboundUpstream(prof config.ProviderProfile) config.Upstream {
	upstreams := prof.ResolvedUpstreams()
	if len(upstreams) == 0 {
		return config.Upstream{}
	}
	u := upstreams[0]
	if u.BaseURL == "" {
		u.BaseURL = prof.BaseURL
	}
	if u.APIKey == "" {
		u.APIKey = prof.APIKey
	}
	if u.Headers == nil {
		u.Headers = prof.Headers
	}
	return u
}

func setOutboundHeader(headers map[string]string, key, value string) {
	for existing := range headers {
		if strings.EqualFold(existing, key) {
			delete(headers, existing)
			break
		}
	}
	headers[key] = value
}

func resolveUserAgentValues(profile, global *string) string {
	if profile != nil && *profile != "" {
		return *profile
	}
	if global != nil && *global != "" {
		return *global
	}
	return "curl/8.5.0"
}

func applyOpenCodeSessionAffinityHeader(baseURL string, incoming, outgoing http.Header) {
	if !strings.Contains(strings.ToLower(baseURL), "opencode.ai") {
		return
	}
	sessionID := incoming.Get("x-opencode-session")
	if sessionID == "" {
		sessionID = incoming.Get("x-session-affinity")
	}
	if sessionID == "" {
		sessionID = incoming.Get("x-client-request-id")
	}
	if sessionID != "" {
		outgoing.Set("x-opencode-session", sessionID)
	}
	if client := incoming.Get("x-opencode-client"); client != "" {
		outgoing.Set("x-opencode-client", client)
	}
}

func buildUpstreamURL(base, path string) string {
	u := strings.TrimRight(base, "/")
	if strings.HasSuffix(u, "/v1") {
		return u + strings.TrimPrefix(path, "/v1")
	}
	return u + path
}
