# Architecture V2 W05b — Usage and cost facts — 2026-10-13

**Status:** W05b passed locally on top of W05a `402fef1`. This is a pipeline foundation, not completion of W05. No push or remote CI run.

## Implementation

- Moved token-usage extraction, unknown/explicit-zero tracking, SSE usage parsing, and cost calculation into `internal/domain/usage`, using the domain Provider model for rates.
- Kept `internal/usage` and `internal/proxy` as thin compatibility facades so existing server and translator callers retain their current interfaces while delegating to one implementation.
- Moved the usage and cost unit tests to the domain package; server request-log and translator tests continue to exercise the compatibility paths.

## Verification

| Check | Result |
|---|---|
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (`desktop/`) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (root and `desktop/`) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |

## Remaining limits

- Inference orchestration still consumes the compatibility facades; moving Result/Usage/Cost and Stream coordination into the inference usecase remains required by W05.
- No live upstream, user config, request history, streaming provider, or remote CI was used.
