package inference

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

const streamErrorBodyLimit = 64 << 10

type StreamOutcomeKind uint8

const (
	StreamOutcomeError StreamOutcomeKind = iota + 1
	StreamOutcomeUpstreamResponse
	StreamOutcomeRelayed
	StreamOutcomeCanceled
)

type StreamOutcome struct {
	Kind      StreamOutcomeKind
	Headers   http.Header
	Body      []byte
	URL       string
	ErrorType string
	ShouldLog bool
	Facts     StreamFacts
}

func ExecuteStream(ctx context.Context, prepared PreparedRequest, build OutboundRequestBuilder, sink http.ResponseWriter) StreamOutcome {
	body, _ := json.Marshal(prepared.Body)
	outbound, err := build(body)
	if err != nil {
		return StreamOutcome{Kind: StreamOutcomeError, ErrorType: "upstream_error", Facts: StreamFacts{Status: http.StatusBadGateway, Error: err.Error()}}
	}
	response, err := outbound.Client.Do(outbound.Request)
	if err != nil {
		kind := StreamOutcomeError
		status := http.StatusBadGateway
		if ctx.Err() != nil {
			kind, status = StreamOutcomeCanceled, 499
		}
		return StreamOutcome{
			Kind: kind, URL: outbound.URL, ErrorType: "upstream_error", ShouldLog: true,
			Facts: StreamFacts{Status: status, Error: err.Error()},
		}
	}
	if response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(response.Body, streamErrorBodyLimit))
		_ = response.Body.Close()
		return StreamOutcome{
			Kind: StreamOutcomeUpstreamResponse, Headers: response.Header, Body: body,
			URL: outbound.URL, ShouldLog: true,
			Facts: StreamFacts{Status: response.StatusCode, Error: string(body)},
		}
	}
	facts := RelayStream(ctx, StreamInput{
		UpstreamStatus:   response.StatusCode,
		UpstreamHeaders:  response.Header,
		Body:             response.Body,
		UpstreamFormat:   prepared.ProtocolPlan.To,
		DownstreamFormat: prepared.ProtocolPlan.From,
		Converter:        prepared.ProtocolPlan.StreamConverter(prepared.Model),
		ModelEntry:       prepared.ModelEntry,
	}, sink)
	return StreamOutcome{Kind: StreamOutcomeRelayed, URL: outbound.URL, ShouldLog: true, Facts: facts}
}
