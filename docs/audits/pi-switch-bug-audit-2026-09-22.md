# pi-switch Bug Audit

Date: 2026-09-22  
Audited revision: 8e60ce14f896697c7954c64004c8df80c1a661fa (main)  
Scope: request routing and translation, streaming, profile mutation, configuration persistence, and request cancellation.

## Executive result

The audit found six actionable defects. Four affect request correctness or resource usage and should be treated as release blockers for the affected paths. Two affect configuration integrity under normal multi-request or multi-process use.

| ID | Severity | Area | Result |
|---|---|---|---|
| PS-001 | P1 | Profile rename | Existing profile can be overwritten; active profile pointer can become stale |
| PS-002 | P1 | Non-stream translation | Some converted requests return the upstream protocol unchanged |
| PS-003 | P1 | Streaming translation | Anthropic/Chat converted streams are silently passed through |
| PS-004 | P1 | Tool-call translation | Chat-to-Responses loses tool-call history and tool results |
| PS-005 | P2 | Request cancellation | Upstream request is detached from the client context |
| PS-006 | P2 | Config persistence | Concurrent read-modify-write operations can lose updates |

The audit is static. No implementation changes were made and no build or test command was run. The findings are based on the code at the audited revision and should be converted into regression tests before implementation work begins.

## Findings

### PS-001 — Rename can overwrite an existing profile and stale current

Evidence: internal/server/profile_handlers.go, handlePutProfile, the rename branch deletes renameFrom, then assigns cfg.Profiles[name] = prof without checking whether name already exists. The same handler does not update cfg.Current when the renamed profile was active.

Reproduction:

1. Create profiles alpha and beta with different API keys.
2. Set current to alpha.
3. Update alpha with renameFrom=alpha and name=beta.
4. The response is successful, beta now contains alpha data, the original beta is lost, and current still points to alpha.

Impact: destructive configuration loss, invalid active-profile state, and possible routing to a nonexistent supplier.

Required behavior: a rename to an existing name must fail atomically with no file change. A successful rename must move current from the old name to the new name when applicable.

### PS-002 — Converted non-stream responses can use the wrong protocol

Evidence: internal/translator/registry.go registers Chat → Responses with a response transform that returns the upstream map unchanged, and Anthropic → Chat with the same pass-through response. Plan.TransformResponse is consequently a no-op for those directions even though the request was converted.

Reproduction:

1. Configure an openai-responses upstream and send a Chat Completions request.
2. Configure an openai-completions upstream and send an Anthropic Messages request.
3. Inspect the response body. It remains in the upstream response schema rather than the client-facing schema.

Impact: compatible clients can fail to parse successful responses, lose usage/content fields, or misinterpret a response as an error.

Required behavior: every accepted request direction must have a matching response conversion, or the capability validator must reject that direction before contacting the upstream. A request must never be converted one way and returned in the other protocol by accident.

### PS-003 — Anthropic/Chat streaming is passed through without conversion

Evidence: internal/translator/registry.go registers stream converters only for Chat ↔ Responses. Plan.StreamConverter returns nil for Anthropic ↔ Chat, and handleStream calls streamPassthrough in that case.

Reproduction:

1. Send POST /v1/messages with stream=true to an OpenAI Chat upstream, or send a Chat request to an Anthropic upstream.
2. Observe that the proxy emits the upstream SSE event format directly.

Impact: the client receives invalid event names and payloads. Streaming may appear to hang, produce no text, or fail after the first event.

Required behavior: implement both streaming directions with terminal-event and usage handling, or reject unsupported protocol pairs before opening the upstream stream. Silent pass-through is invalid.

### PS-004 — Chat-to-Responses drops tool-call state

Evidence: internal/translator/chat_anthropic.go, ChatToResponses, maps each non-system message to only role and normalized content. It does not preserve Chat tool_calls, tool_call_id, function names, or arguments. tools is copied separately but does not restore the conversation history.

Reproduction:

1. Send a Chat request containing an assistant message with tool_calls.
2. Include the subsequent tool result message with tool_call_id.
3. Inspect the generated Responses input. The assistant tool call and tool result are absent or reduced to incomplete content.

Impact: multi-turn tool use cannot continue reliably through a Responses upstream; the model may repeat a call, reject the request, or answer without the tool result.

Required behavior: preserve the semantic tool-call sequence and map it to Responses function-call and function-call-output items. Unsupported tool shapes must produce a clear 4xx/502 translation error rather than data loss.

### PS-005 — Client cancellation does not cancel upstream work

Evidence: internal/server/outbound.go, BuildOutboundRequest, uses http.NewRequest instead of http.NewRequestWithContext. The request plan carries no context. Streaming uses an unlimited client timeout and ignores downstream writer errors.

Reproduction:

1. Start a slow streaming request.
2. Close the client connection before the upstream finishes.
3. Observe that the upstream request remains active until its own completion or network timeout.

Impact: abandoned generations consume upstream quota, occupy connections, and can accumulate under repeated client disconnects.

Required behavior: derive the upstream request from c.Request.Context(), stop reading when that context is cancelled, close the upstream body, and record cancellation distinctly from upstream failure.

### PS-006 — Concurrent config writes can lose updates

Evidence: profile and settings handlers independently load the complete config, mutate one field, and call config.SaveAtPath. Atomic rename protects file integrity but does not serialize read-modify-write transactions or detect stale versions. The same pattern is used by CLI and server entry points.

Reproduction:

1. Start two writes from the same config snapshot, such as adding a profile and changing settings.
2. Delay both before save so they overlap.
3. The later save replaces the earlier mutation while both callers report success.

Impact: silent configuration loss under simultaneous WebUI actions, CLI use, or multiple processes.

Required behavior: serialize mutations across processes or reject stale writes with a conflict. The transaction must preserve unrelated fields and keep atomic replacement semantics.

## Review limits and open questions

The audit does not claim that every translator is semantically complete for every provider-specific field. The implementation plan therefore requires an explicit capability matrix and tests for accepted pairs. Unsupported pairs must fail before upstream I/O. The plan also requires testing both HTTP server concurrency and separate-process writers because an in-process mutex alone would not solve PS-006.

## Recommended delivery order

1. PS-001 profile rename safety.
2. PS-002 response conversion contract.
3. PS-003 streaming capability gate and converters.
4. PS-004 tool-call preservation.
5. PS-005 cancellation propagation.
6. PS-006 cross-process config transactions.
7. Full validation and release review.

