# Architecture V2 W01 — Provider mutation use case — 2026-10-13

**Status:** W01 acceptance passed locally. Previous ticket: W00 `0a89686`. No push, merge, live-data access, or manual GUI claim.

## Implementation

- `internal/domain/provider` now owns Provider/Channel/Model value types and their JSON/value behavior. `internal/config` retains type aliases and `MergeHeaders` compatibility, so existing callers and config JSON stay on one representation.
- `internal/usecase/providers` exposes typed commands/results and separate legacy HTTP versus revisioned Native Save/Delete operations. Shared Store and Validator ports keep rename/current, backup, and revision policies explicit.
- `internal/adapters/outbound/fileconfig` adapts the use case to the existing `config.UpdateAtPath` cross-process lock and the shared pre-write backup implementation. `internal/adapters/outbound/profilevalidation` delegates to the existing `internal/profile.ValidateProfile`; CLI, HTTP, and Native therefore still have one field-validation source.
- `internal/bootstrap` wires those adapters. Existing `internal/application` entrypoints remain thin compatibility facades for Desktop/server callers. HTTP handlers now only parse/map the legacy `/api/profiles/:name` response; their mutation closures are in the use case.
- W01 bridge imports are explicitly scoped in `internal/archcheck`: only `outbound/fileconfig` can import legacy `internal/config`, and only `outbound/profilevalidation` can import legacy `internal/profile`.

## Compatibility evidence

The W00 fixtures were run against the new path and still prove:

- HTTP PUT/DELETE status and response JSON remain unchanged; exact serialized config bytes match the legacy mutation, including HTTP rename leaving `current` unchanged; HTTP writes still make no backup and require no revision.
- HTTP validation remains before storage; missing DELETE remains 404 with zero config-byte changes.
- Native Save/Delete still check optimistic revisions, preserve the exact pre-write bytes in a mode-0600 backup, and update/clear `current` on rename/delete.
- Concurrent HTTP PUT and Native Save retain both profile updates; the integration test passed under the race detector for 10 repetitions.

## Verification

Commands used isolated temporary config/database/models/session paths and `GOTOOLCHAIN=local GOPROXY=off` where relevant.

| Check | Result |
|---|---|
| `go test -mod=readonly ./...` (root module) | PASS |
| `go vet -mod=readonly ./...` (root module) | PASS |
| `go test -mod=readonly ./...` (`desktop` module) | PASS |
| `go vet -mod=readonly ./...` (`desktop` module) | PASS |
| `go test -race ./internal/server -run '^TestHTTPAndNativeProviderMutationsDoNotLoseConcurrentUpdates$' -count=10` | PASS |
| `go run ./internal/archcheck` and checker fixture tests | PASS — clean legacy tree; forbidden-import fixtures rejected |

## Remaining limits

- `fileconfig.Store` checks `ctx` before using the current `config.UpdateAtPath`; lock waiting itself is not cancellable. This is recorded as a `shortcut:` and belongs to W02's file-lock/context work.
- Desktop/server still use the `internal/application` compatibility facade; entrypoint wiring cleanup remains W08. No manual GUI smoke or remote CI was run for this ticket.
