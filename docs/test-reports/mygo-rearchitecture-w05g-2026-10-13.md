# Architecture V2 W05g — non-stream outbound execution — 2026-10-13

**Status:** W05g passed locally on top of W05f `c8d469d`. W05 as a whole remains incomplete. No push or remote CI run.

## Implementation

- Added `inference.ExecuteNonStream` to coordinate initial outbound dispatch, response reading, the existing single 400 `invalid_request_error` retry, conversion, and success/error/usage/cost facts.
- Kept HTTP request construction in the server adapter via a narrow builder callback. The handler maps the returned outcome to Gin responses and persists request facts.
- Preserved existing distinctions: initial request-build errors are not logged; transport/read errors are logged; cancellation returns no response with status 499; failed retries fall back to the original upstream error response; successful retries use their own status and URL.
- Added usecase coverage for retry-body contents, retry success, build/HTTP/transport retry failures, initial build failure, and canceled retry. Existing HTTP tests still cover session-affinity headers and request behavior.

## Verification

| Check | Result |
|---|---|
| Targeted inference and server request/retry/protocol/cancellation tests | PASS |
| Targeted `go test -race` (inference execution and server request/retry paths) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |

## Remaining limits

- W05 is still incomplete: streaming upstream dispatch/error adaptation, HTTP response writing, and request persistence/logging remain in `internal/server`. Non-stream dispatch/retry/result coordination now lives in the inference usecase.
- W06–W08 and final CI/secrets/smoke gates remain outstanding. No live upstream, GUI, or real user data was used for this slice.
