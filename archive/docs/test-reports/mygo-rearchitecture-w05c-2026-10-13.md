# Architecture V2 W05c — Protocol plans and conversion rules — 2026-10-13

**Status:** W05c passed locally on top of W05b `cd16386`. This migrates the protocol rules, not the complete inference coordinator. No push or remote CI run.

## Implementation

- Moved request-plan selection, Chat/Responses/Messages body conversion, response conversion, tool-history handling, and SSE event converters into `internal/domain/protocol`, which now owns the protocol capability rules and conversion implementation.
- Domain protocol conversion depends only on the domain usage facts package; it no longer imports the legacy `internal/protocol`, `internal/usage`, or `internal/translator` packages.
- Updated the HTTP handler to consume the domain protocol package directly and moved the translator regression tests with the implementation. Existing route/error, response, tool, stream, cancellation, and usage integration coverage remains in the root suite.

## Verification

| Check | Result |
|---|---|
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (`desktop/`) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (root and `desktop/`) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |

## Remaining limits

- `internal/server` still coordinates route, request planning, outbound execution, result mapping, usage persistence, and stream writing. Extracting those workflows into `internal/usecase/inference` and leaving HTTP adaptation thin remains required by W05.
- No live upstream, manual Proxy smoke, or remote CI was used; race validation of the complete inference path remains pending.
