# Architecture V2 W10 — Inference / Proxy Adapter — 2026-10-14

**Status:** W10 acceptance passed locally. The inference use case no longer depends on `net/http` request/response or `http.ResponseWriter`; HTTP request construction, execution, response conversion, and Gin response-writer adaptation live in `internal/adapters/outbound/proxy`. No push or remote CI run was performed.

## Scope

- Added a transport-neutral inference seam:
  - `inference.OutboundRequest` executes a request through an injected adapter.
  - `inference.OutboundResponse` carries status, headers, body, and URL without HTTP types.
  - `inference.StreamWriter` keeps SSE relay output independent of Gin/HTTP types.
- Moved the outbound HTTP policy from `internal/server` to `internal/adapters/outbound/proxy`:
  - URL joining, authorization, merged profile/channel headers, user-agent fallback, OpenCode affinity headers, timeout, cancellation context, and HTTP response adaptation remain in one adapter.
  - The server now wires the adapter into the inference use case and keeps request logging/HTTP response selection unchanged.
- Moved the user-agent rule to `internal/domain/config`; the legacy `internal/config` function remains a compatibility delegate.
- Kept and relocated outbound policy tests, retained server-level initial/retry/stream header coverage, and converted inference tests to fake transport/writer implementations.

## Behavior contract

The existing implementation and regression suite continue to protect:

- non-stream invalid-request retry and min-token retry body;
- retry failure fallback to the original upstream response;
- request cancellation as status `499` with request logging;
- SSE pass-through and conversion, upstream status/header relay, terminal-event handling, bounded upstream errors, downstream write failure, cancellation, usage extraction, and cost calculation;
- outbound URL, authorization, header precedence, user-agent fallback, OpenCode session affinity, timeout, and identical policy across initial request, retry, and stream.

No retry, SSE, cancellation, or billing/usage calculation rule was changed; only the transport seam and adapter location changed.

## Verification

All stateful checks used the repository's isolated test fixtures; no real user config, credentials, database, model, or session data was used.

| Check | Result |
|---|---|
| Focused inference, proxy adapter, and server tests | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` | PASS |
| `CGO_ENABLED=0 go -C desktop test -mod=readonly ./...` | PASS |
| `CGO_ENABLED=0 go -C desktop vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run -mod=readonly ./internal/archcheck` | PASS — `architecture dependencies: clean` |
| `git diff --check` for W10 paths | PASS |

## Acceptance and remaining risk

W10 is accepted locally: inference production files contain no `net/http` dependency, HTTP details are isolated in the outbound Proxy Adapter, the architecture checker passes, and root/Desktop tests and vet pass.

Remaining risk is verification scope, not a known W10 failure: GitHub Actions/CI runner differences, native arm64 execution, and Arch+niri/Wayland/IME/hardware-scaling GUI gates were not run here. Those remain later release/environment gates.
