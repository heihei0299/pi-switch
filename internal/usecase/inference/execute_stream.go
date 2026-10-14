package inference

import (
	"context"
	"encoding/json"
	"io"
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
	Headers   Header
	Body      []byte
	URL       string
	ErrorType string
	ShouldLog bool
	Facts     StreamFacts
}

func ExecuteStream(ctx context.Context, prepared PreparedRequest, build OutboundRequestBuilder, start StreamResponseStarter, sink StreamSink) StreamOutcome {
	body, _ := json.Marshal(prepared.Body)
	outbound, err := build(body)
	if err != nil {
		return StreamOutcome{Kind: StreamOutcomeError, ErrorType: "upstream_error", Facts: StreamFacts{Status: 502, Error: err.Error()}}
	}
	response, err := outbound.Do(ctx)
	if err != nil {
		kind := StreamOutcomeError
		status := 502
		if ctx.Err() != nil {
			kind, status = StreamOutcomeCanceled, 499
		}
		return StreamOutcome{
			Kind: kind, URL: response.URL, ErrorType: "upstream_error", ShouldLog: true,
			Facts: StreamFacts{Status: status, Error: err.Error()},
		}
	}
	if response.Status >= 400 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, streamErrorBodyLimit))
		_ = response.Body.Close()
		return StreamOutcome{
			Kind: StreamOutcomeUpstreamResponse, Headers: response.Headers, Body: body,
			URL: response.URL, ShouldLog: true,
			Facts: StreamFacts{Status: response.Status, Error: string(body)},
		}
	}
	if start != nil {
		start(response.Status, response.Headers)
	}
	facts := RelayStream(ctx, StreamInput{
		UpstreamStatus:   response.Status,
		Body:             response.Body,
		UpstreamFormat:   prepared.ProtocolPlan.To,
		DownstreamFormat: prepared.ProtocolPlan.From,
		Converter:        prepared.ProtocolPlan.StreamConverter(prepared.Model),
		ModelEntry:       prepared.ModelEntry,
	}, sink)
	return StreamOutcome{Kind: StreamOutcomeRelayed, URL: response.URL, ShouldLog: true, Facts: facts}
}
