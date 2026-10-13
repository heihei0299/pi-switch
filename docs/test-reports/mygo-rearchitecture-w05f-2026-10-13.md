# Architecture V2 W05f — non-stream result facts extraction — 2026-10-13

**Status:** W05f passed locally on top of W05e `328f411`. W05 as a whole remains incomplete. No push or remote CI run.

## Implementation

- Added `inference.ProcessNonStreamResponse` to translate successful upstream response bodies back to the client protocol and return usage/cost facts.
- Reused it for both the ordinary successful response and the existing manual 400-retry success path. The HTTP handler still owns upstream dispatch, status/error mapping, response headers/body writing, and request persistence.
- Preserved passthrough bytes (including non-JSON bodies), conversion error messages, unknown-usage behavior, and cost calculation semantics. No retry conditions or outbound behavior changed.
- Added usecase tests for Responses-to-Chat conversion, usage/cost, passthrough/unknown usage, and malformed converted responses.

## Verification

| Check | Result |
|---|---|
| Targeted inference and server protocol/retry tests | PASS |
| Targeted `go test -race` (inference result and server result/retry paths) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |

## Remaining limits

- W05 is still incomplete: upstream request construction/dispatch, retry workflow, and HTTP response/error adaptation remain in `internal/server`. Usage facts and non-stream response conversion now live in the inference usecase; request persistence remains an adapter concern.
- W06–W08 and final CI/secrets/smoke gates remain outstanding. No live upstream, GUI, or real user data was used for this slice.
