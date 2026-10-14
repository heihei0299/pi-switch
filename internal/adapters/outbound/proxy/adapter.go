package proxy

import (
	"bytes"
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	domainconfig "github.com/heihei0299/pi-switch/internal/domain/config"
	"github.com/heihei0299/pi-switch/internal/domain/provider"
	"github.com/heihei0299/pi-switch/internal/usecase/inference"
)

type OutboundRequestPlan struct {
	Upstream         provider.Upstream
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
	for key, value := range provider.MergeHeaders(plan.ProfileHeaders, plan.Upstream.Headers) {
		req.Header.Set(key, value)
	}
	req.Header.Set("User-Agent", domainconfig.ResolveUserAgentValues(plan.ProfileUserAgent, plan.GlobalUserAgent))
	ApplyOpenCodeSessionAffinityHeader(plan.Upstream.BaseURL, plan.IncomingHeaders, req.Header)

	headerNames := make([]string, 0, len(req.Header))
	for name := range req.Header {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	return BuiltOutboundRequest{
		Request: req,
		Client:  &http.Client{Timeout: plan.Timeout},
		Metadata: OutboundRequestMetadata{
			URL: url, Method: method, HasAuthorization: plan.Upstream.APIKey != "", HeaderNames: headerNames,
		},
	}, nil
}

func OutboundRequestBuilder(plan OutboundRequestPlan) inference.OutboundRequestBuilder {
	return func(body []byte) (inference.OutboundRequest, error) {
		requestPlan := plan
		requestPlan.Body = body
		outbound, err := BuildOutboundRequest(requestPlan)
		if err != nil {
			return nil, err
		}
		return httpOutboundRequest{request: outbound.Request, client: outbound.Client, url: outbound.Metadata.URL}, nil
	}
}

type httpOutboundRequest struct {
	request *http.Request
	client  *http.Client
	url     string
}

func (r httpOutboundRequest) Do(ctx context.Context) (inference.OutboundResponse, error) {
	request := r.request.Clone(ctx)
	response, err := r.client.Do(request)
	if err != nil {
		return inference.OutboundResponse{URL: r.url}, err
	}
	return inference.OutboundResponse{
		Status:  response.StatusCode,
		Headers: fromHTTPHeaders(response.Header),
		Body:    response.Body,
		URL:     r.url,
	}, nil
}

func fromHTTPHeaders(headers http.Header) inference.Header {
	out := make(inference.Header, len(headers))
	for key, values := range headers {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func NewStreamWriter(w http.ResponseWriter) inference.StreamSink {
	return streamWriter{writer: w}
}

type streamWriter struct{ writer http.ResponseWriter }

func (w streamWriter) Write(data []byte) (int, error) { return w.writer.Write(data) }
func (w streamWriter) Flush() {
	if flusher, ok := w.writer.(http.Flusher); ok {
		flusher.Flush()
	}
}

func NewStreamResponseStarter(w http.ResponseWriter) inference.StreamResponseStarter {
	return func(status int, headers inference.Header) {
		for key, values := range headers {
			for _, value := range values {
				if strings.EqualFold(key, "Content-Length") || strings.EqualFold(key, "Transfer-Encoding") || strings.EqualFold(key, "Connection") {
					continue
				}
				w.Header().Set(key, value)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(status)
	}
}

func WriteResponse(w http.ResponseWriter, status int, headers inference.Header, body []byte) {
	for key, values := range headers {
		for _, value := range values {
			w.Header().Set(key, value)
		}
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func ApplyOpenCodeSessionAffinityHeader(baseURL string, incoming, outgoing http.Header) {
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
