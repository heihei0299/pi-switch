# Architecture V2 W05d — SSE relay extraction — 2026-10-13

**Status:** W05d stream-relay slice passed locally on top of W05c `8aa9db2`. W05 as a whole is still incomplete; non-stream request orchestration and the broader route/plan/outbound/result coordinator remain in `internal/server`. No push or remote CI run.

## Implementation

- Moved SSE frame parsing, pass-through, protocol conversion, flush/backpressure, terminal/failure detection, cancellation handling, usage extraction, and cost calculation into `internal/usecase/inference.RelayStream`.
- The HTTP handler sets the upstream status on the response, passes upstream response data plus the writer to the usecase, then persists the returned success/status/error/usage/cost facts with the existing request logger.
- Kept streaming synchronous: each chunk/event is written and flushed before reading further; downstream write failure closes the upstream body. No retry/failover scheduler was wired or changed.
- Added usecase tests for terminal/missing-terminal/failure events, unknown usage, cancellation, downstream write failure, headers, and flush cadence. Existing HTTP tests continue to cover real Responses-to-Chat streaming and request-fact persistence.

## Retry/failover evidence

- The requested test names `TestSingleCandidateNoRetryOrCooling` and `TestPassthroughUsesOnlyFirstCandidate` do not exist in this checkout.
- The actual test `internal/server/passthrough_single_test.go::TestPassthrough_429Isolated` verifies two immediate non-stream 429 requests hit supplier A and not B; it now also verifies a stream 429 leaves counts at A=3, B=0.
- `TestMultiSupplier_FailoverAndHotUpdate` exists but is skipped with `failover removed, test skipped`; it is not passing evidence.
- `retry.go`'s `DORMANT (scheduling only)` note is unchanged. The live stream path still selects `candidates[0]`; its channel narrowing remains live, while the scheduler is not called from the request handler.

## Verification

| Check | Result |
|---|---|
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (`desktop/`) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (root and `desktop/`) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run ./internal/archcheck` | PASS — clean |
| Targeted `go test -race` (`internal/usecase/inference` relay tests and server stream/cancellation/429 tests) | PASS |
| `git diff --check` | PASS |

## Remaining limits

- W05's broader scope is not complete: config/body intake, route and request planning, outbound coordination, and non-stream result/log coordination still live in the HTTP server handler. This commit only extracts the already-tested SSE relay slice.
- No live upstream or user data was used.
