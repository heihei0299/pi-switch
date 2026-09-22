# pi-switch Bug Fix Execution Plan

Base revision: 8e60ce14f896697c7954c64004c8df80c1a661fa  
Source audit: docs/audits/pi-switch-bug-audit-2026-09-22.md

This plan is written for another implementation agent. It covers only the six confirmed findings in the audit. Do not broaden the work into unrelated refactors or provider-specific enhancements.

## Delivery rules

- One ticket produces one complete delivery commit.
- A ticket is complete only when its regression tests and affected existing tests pass.
- Do not make a separate tests commit or a formatting-only commit.
- Keep the public API and config format unchanged unless a ticket explicitly requires a new error or capability declaration.
- Run the repository limited validation entry points. Do not run unrestricted parallel builds or tests.
- If a protocol pair cannot be implemented safely in the ticket, reject it before upstream I/O and document the capability in the existing protocol source of truth.

## Dependency graph

~~~text
PS-001 rename safety ───────┐
PS-002 response contract ───┼──> final validation
PS-003 streaming contract ──┤
PS-004 tool calls ───────────┤
PS-005 cancellation ─────────┤
PS-006 config transaction ───┘
~~~

PS-002 should land before PS-003 and PS-004 because both use the same inbound/outbound protocol matrix. PS-006 can be implemented after PS-001 because rename correctness depends on a safe mutation boundary.

## Ticket PS-001 — Make profile rename atomic and non-destructive

Goal: prevent overwrite of an existing profile and keep current consistent after a successful rename.

Likely files: internal/server/profile_handlers.go, internal/profile/profile.go or a shared profile mutation helper, and existing profile/server tests.

Implementation:

1. Validate renameFrom and target names before mutating the in-memory config.
2. If renameFrom != name and the target already exists, return the existing conflict-style 4xx response used for duplicate profiles.
3. Apply delete, insert, and current pointer update to one copied config value.
4. Save once through the canonical config save path.
5. Ensure any save failure leaves the original file and in-memory request state unchanged.

Regression tests:

- rename to an existing target returns conflict and preserves both profiles byte-for-byte;
- rename an active profile updates current;
- rename a non-active profile leaves current unchanged;
- save failure does not partially remove the source profile.

Acceptance: no profile is lost, no stale current remains, and one request produces at most one config write.

Commit: fix(profile): make rename conflict-safe and update current

## Ticket PS-002 — Enforce complete non-stream protocol conversion

Goal: ensure every accepted converted request returns the client-facing response schema.

Likely files: internal/translator/registry.go, internal/translator/translator.go, protocol capability validation, and translator/server tests.

Implementation:

1. Enumerate supported inbound/outbound pairs in one table derived from the existing protocol capability source.
2. For Chat → Responses, implement Responses → Chat response conversion or explicitly reject the pair if the project contract does not support it.
3. For Anthropic → Chat, implement Chat → Anthropic response conversion or reject the pair before sending the request.
4. Make PlanRequest and TransformResponse use the same pair table so a request cannot be accepted without a response path.
5. Preserve model id, text, finish reason, tool calls, usage, and error shape where those fields exist.

Regression tests:

- table-driven test for every accepted pair;
- successful non-stream response round trips for Chat, Responses, and Anthropic;
- unsupported pair fails before the test upstream receives a request;
- usage and finish fields survive conversion;
- upstream error responses retain the inference error envelope.

Acceptance: an accepted non-stream request always returns the protocol declared by its inbound endpoint; unsupported pairs fail deterministically before upstream I/O.

Commit: fix(translator): require complete non-stream response conversion

## Ticket PS-003 — Close the streaming conversion gap

Goal: prevent Anthropic/Chat streams from being silently passed through in the wrong format.

Likely files: internal/translator stream converters and internal/server/proxy_handlers.go; add focused SSE fixtures under existing translator/server tests.

Implementation:

1. Add Chat SSE → Anthropic SSE conversion and Anthropic SSE → Chat SSE conversion, including text deltas, tool events, terminal events, and usage where supported.
2. If a direction cannot preserve semantics, remove it from the accepted capability matrix and return a clear inference error before opening the stream.
3. Make Plan.StreamConverter distinguish same-format passthrough from missing converter. Missing converter must never fall back to passthrough.
4. Verify terminal detection and failure logging for each accepted stream format.

Regression tests:

- fragmented SSE frames across read boundaries;
- normal text stream reaches the correct terminal event;
- stream failure and early EOF are reported correctly;
- tool-call stream fixtures for every accepted direction;
- unsupported direction produces no upstream request.

Acceptance: the proxy either emits valid client-facing events or rejects the pair before upstream I/O. It never relays an incompatible SSE protocol as if it were valid.

Commit: fix(translator): reject or convert unsupported streaming pairs

## Ticket PS-004 — Preserve tool calls in Chat-to-Responses conversion

Goal: preserve multi-turn tool-call semantics when Chat input is converted to Responses input.

Likely files: internal/translator/chat_anthropic.go, internal/translator/translator.go, and translator tests.

Implementation:

1. Map assistant tool_calls to Responses function_call items with stable call ids, names, and JSON arguments.
2. Map tool messages with tool_call_id to Responses function_call_output items.
3. Preserve multiple calls in their original order and preserve ordinary text parts around them.
4. Map tool_choice and tool definitions only where the target protocol supports the same meaning; otherwise return a typed conversion error.
5. Ensure reverse response conversion maps function calls back to Chat tool_calls with valid ids and arguments.

Regression tests:

- one assistant function call followed by one tool result;
- multiple parallel tool calls and results;
- mixed text plus tool call content;
- malformed arguments and missing ids return errors without partial output;
- non-tool conversations remain semantically compatible with current behavior.

Acceptance: a two-turn tool exchange reaches the Responses upstream with all calls and results intact and returns a valid Chat response.

Commit: fix(translator): preserve tool calls across responses conversion

## Ticket PS-005 — Propagate request cancellation to upstream

Goal: stop upstream work when the client disconnects or the request context is cancelled.

Likely files: internal/server/outbound.go, internal/server/proxy_handlers.go, and outbound/stream tests.

Implementation:

1. Add a request context to OutboundRequestPlan and build requests with http.NewRequestWithContext.
2. Pass c.Request.Context() from both non-stream and stream paths.
3. In streaming relays, stop reading and close the upstream body when the context is done.
4. Check downstream write errors and classify client cancellation separately from upstream failure.
5. Keep the configured finite timeout for non-stream requests and define an explicit stream lifetime policy rather than relying on an unlimited client timeout.

Regression tests:

- cancellation closes a blocking upstream handler;
- client disconnect does not leave a goroutine or response body open;
- normal EOF still records success;
- upstream timeout remains classified as an upstream failure;
- cancellation does not trigger retry or misleading 502 output.

Acceptance: after client cancellation, the upstream request is cancelled promptly and the request log records the cancellation outcome without charging a successful completion.

Commit: fix(server): propagate client cancellation to upstream requests

## Ticket PS-006 — Serialize config mutations across processes

Goal: eliminate lost updates while preserving atomic file replacement and existing config migration behavior.

Likely files: internal/config/config.go, profile/settings mutation callers, and config concurrency tests.

Implementation:

1. Add one canonical read-modify-write transaction API at the config boundary.
2. Use an advisory lock file in the config directory, with a bounded acquisition failure path and cleanup that does not delete another process lock.
3. Re-read the config after acquiring the lock, apply the mutation callback, run existing save-time migration, and atomically replace the config.
4. Route profile, settings, model/expose, duplicate, delete, and CLI mutations through this API.
5. Keep read-only handlers lock-free and preserve the existing 0600 mode and parent-directory behavior.
6. Add a version/conflict check only if the chosen locking mechanism cannot cover all supported writers.

Regression tests:

- two goroutines updating disjoint fields preserve both updates;
- two separate processes updating disjoint fields preserve both updates;
- lock acquisition timeout returns an explicit error and leaves the config unchanged;
- save failure leaves the previous file intact;
- migration and file mode tests continue to pass.

Acceptance: concurrent writers cannot silently overwrite unrelated changes, and all mutation entry points use the same transaction boundary.

Commit: fix(config): serialize cross-process config mutations

## Final validation

After all six commits, run the repository-provided limited suites once:

~~~bash
scripts/test-limited.sh go webui tsc smoke
~~~

Also run targeted tests while developing each ticket, then inspect:

~~~bash
git diff --check
git status --short
git log --oneline --decorate -8
~~~

Manual acceptance should cover profile rename, one non-stream request for each supported protocol pair, one streamed request for each supported pair, a tool-call round trip, client disconnect, and simultaneous WebUI/CLI config writes. Record pre-existing failures separately from failures introduced by these tickets.

