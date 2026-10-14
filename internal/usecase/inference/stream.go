package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/heihei0299/pi-switch/internal/domain/protocol"
	"github.com/heihei0299/pi-switch/internal/domain/provider"
	"github.com/heihei0299/pi-switch/internal/domain/usage"
)

// StreamInput describes the upstream response and the conversion plan for one SSE relay.
type StreamInput struct {
	UpstreamStatus   int
	UpstreamHeaders  Header
	Body             io.ReadCloser
	UpstreamFormat   protocol.Format
	DownstreamFormat protocol.Format
	Converter        protocol.StreamEventConverter
	ModelEntry       *provider.ModelEntry
}

// StreamFacts are the status and usage facts to persist after a relay.
type StreamFacts struct {
	Success bool
	Status  int
	Error   string
	Usage   *usage.UsageSummary
	Cost    *float64
}

// RelayStream sets upstream headers/status, relays the SSE body, and returns request facts.
func RelayStream(ctx context.Context, input StreamInput, sink StreamWriter) StreamFacts {
	defer input.Body.Close()
	for key, values := range input.UpstreamHeaders {
		for _, value := range values {
			if strings.EqualFold(key, "Content-Length") || strings.EqualFold(key, "Transfer-Encoding") || strings.EqualFold(key, "Connection") {
				continue
			}
			sink.SetHeader(key, value)
		}
	}
	sink.SetHeader("Content-Type", "text/event-stream")
	sink.SetHeader("Cache-Control", "no-cache")
	sink.SetHeader("Connection", "keep-alive")
	sink.WriteHeader(input.UpstreamStatus)
	if input.Converter == nil {
		return relayPassthrough(ctx, input, sink)
	}
	return relayConverted(ctx, input, sink)
}

func relayPassthrough(ctx context.Context, input StreamInput, sink StreamWriter) StreamFacts {
	parser := usage.NewSseUsageParser()
	flusher := sink
	buf := make([]byte, 4096)
	var totalBytes bytes.Buffer
	var frames bytes.Buffer
	terminalSeen := false
	logicalFailure := false
	var readErr error
	clientCanceled := false
	for {
		n, err := input.Body.Read(buf)
		if n > 0 {
			if ctx.Err() != nil {
				readErr = ctx.Err()
				clientCanceled = true
				_ = input.Body.Close()
				break
			}
			chunk := buf[:n]
			parser.Push(chunk)
			totalBytes.Write(chunk)
			consumeSSEFrames(&frames, chunk, func(data string) {
				if isSSETerminal(data, input.UpstreamFormat) {
					terminalSeen = true
				}
				if isSSEFailure(data, input.UpstreamFormat) {
					logicalFailure = true
				}
			})
			if _, writeErr := sink.Write(chunk); writeErr != nil {
				readErr = writeErr
				clientCanceled = true
				_ = input.Body.Close()
				break
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if clientCanceled {
			break
		}
		if err != nil {
			readErr = err
			break
		}
	}
	clientCanceled = clientCanceled || ctx.Err() != nil
	usageSum := parser.Finish()
	if usageSum == nil {
		var response map[string]interface{}
		_ = json.Unmarshal(totalBytes.Bytes(), &response)
		usageSum = usage.ExtractUsage(response)
	}
	streamFailed := clientCanceled || logicalFailure || (readErr != nil && !errors.Is(readErr, io.EOF))
	errorMessage := ""
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		errorMessage = readErr.Error()
	} else if logicalFailure {
		errorMessage = "upstream stream failed"
	} else if !terminalSeen {
		streamFailed = true
		errorMessage = "upstream stream ended without terminal event"
	}
	status := input.UpstreamStatus
	if streamFailed {
		status = 502
	}
	if clientCanceled {
		status = 499
		if ctx.Err() != nil {
			errorMessage = ctx.Err().Error()
		}
	}
	return StreamFacts{Success: !streamFailed, Status: status, Error: errorMessage, Usage: usageSum, Cost: usage.CalcUsageCost(input.ModelEntry, usageSum)}
}

func relayConverted(ctx context.Context, input StreamInput, sink StreamWriter) StreamFacts {
	parser := usage.NewSseUsageParser()
	flusher := sink
	streamFailed := false
	clientCanceled := false
	var streamErr error
	terminalSeen := false
	write := func(line string) {
		if clientCanceled {
			return
		}
		if err := ctx.Err(); err != nil {
			streamFailed, clientCanceled, streamErr = true, true, err
			return
		}
		if _, err := sink.Write([]byte(line)); err != nil {
			streamFailed, clientCanceled, streamErr = true, true, err
			_ = input.Body.Close()
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
	emit := func(event map[string]interface{}) {
		if _, ok := event["error"]; ok {
			streamFailed = true
		}
		if typ, ok := event["type"].(string); ok && typ == "response.failed" {
			streamFailed = true
		}
		b, _ := json.Marshal(event)
		var line string
		if input.DownstreamFormat == protocol.FormatOpenAIResponses {
			typ, _ := event["type"].(string)
			line = fmt.Sprintf("event: %s\ndata: %s\n\n", typ, string(b))
		} else {
			line = fmt.Sprintf("data: %s\n\n", string(b))
		}
		write(line)
	}
	var frames bytes.Buffer
	buf := make([]byte, 4096)
	for {
		n, err := input.Body.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			parser.Push(chunk)
			consumeSSEFrames(&frames, chunk, func(data string) {
				if clientCanceled || data == "" {
					return
				}
				if isSSETerminal(data, input.UpstreamFormat) {
					terminalSeen = true
				}
				if data == "[DONE]" {
					return
				}
				var event map[string]interface{}
				if json.Unmarshal([]byte(data), &event) != nil {
					return
				}
				events, err := input.Converter.PushEvent(event)
				if err != nil {
					streamFailed = true
					streamErr = err
					return
				}
				for _, event := range events {
					if clientCanceled {
						break
					}
					emit(event)
				}
			})
			if clientCanceled {
				break
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				streamFailed = true
				streamErr = err
			}
			break
		}
	}
	if ctx.Err() != nil {
		streamFailed, clientCanceled, streamErr = true, true, ctx.Err()
	}
	if !streamFailed && !terminalSeen {
		streamFailed = true
		streamErr = errors.New("upstream stream ended without terminal event")
	}
	if !streamFailed {
		for _, event := range input.Converter.Finish() {
			if clientCanceled {
				break
			}
			emit(event)
		}
		if input.DownstreamFormat != protocol.FormatOpenAIResponses && !streamFailed {
			write("data: [DONE]\n\n")
		}
	}
	usageSum := parser.Finish()
	if usageSum == nil {
		if raw, ok := input.Converter.UsagePayload().(map[string]interface{}); ok {
			usageSum = usage.ExtractUsage(map[string]interface{}{"usage": raw})
		}
	}
	status := input.UpstreamStatus
	errorMessage := ""
	if streamFailed {
		status = 502
		if streamErr != nil {
			errorMessage = streamErr.Error()
		} else {
			errorMessage = "upstream stream failed"
		}
	}
	if clientCanceled {
		status = 499
	}
	return StreamFacts{Success: !streamFailed, Status: status, Error: errorMessage, Usage: usageSum, Cost: usage.CalcUsageCost(input.ModelEntry, usageSum)}
}

func findFrameEnd(buf []byte) (int, int) {
	for i := 0; i+3 < len(buf); i++ {
		if buf[i] == '\r' && buf[i+1] == '\n' && buf[i+2] == '\r' && buf[i+3] == '\n' {
			return i, 4
		}
	}
	for i := 0; i+1 < len(buf); i++ {
		if buf[i] == '\n' && buf[i+1] == '\n' {
			return i, 2
		}
	}
	return -1, 0
}

func extractData(frame []byte) string {
	lines := strings.Split(string(frame), "\n")
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "data:") {
			d := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if d != "" {
				return d
			}
		}
	}
	return ""
}

func consumeSSEFrames(buf *bytes.Buffer, chunk []byte, onData func(string)) {
	buf.Write(chunk)
	for {
		raw := buf.Bytes()
		end, separator := findFrameEnd(raw)
		if end < 0 {
			return
		}
		frame := append([]byte(nil), raw[:end]...)
		buf.Next(end + separator)
		onData(extractData(frame))
	}
}

func isSSEFailure(data string, format protocol.Format) bool {
	var event map[string]interface{}
	if json.Unmarshal([]byte(data), &event) != nil {
		return false
	}
	typ, _ := event["type"].(string)
	switch format {
	case protocol.FormatOpenAIResponses:
		return typ == "response.failed"
	case protocol.FormatAnthropic:
		return typ == "error"
	default:
		_, ok := event["error"]
		return ok
	}
}

func isSSETerminal(data string, format protocol.Format) bool {
	if format == protocol.FormatOpenAIChat {
		return data == "[DONE]"
	}
	var event map[string]interface{}
	if json.Unmarshal([]byte(data), &event) != nil {
		return false
	}
	typ, _ := event["type"].(string)
	switch format {
	case protocol.FormatOpenAIResponses:
		return typ == "response.completed" || typ == "response.incomplete" || typ == "response.failed"
	case protocol.FormatAnthropic:
		return typ == "message_stop"
	default:
		return false
	}
}
