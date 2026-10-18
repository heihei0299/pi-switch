# Architecture V2 W07b — Settings usecase and save contracts — 2026-10-13

**Status:** Settings slice passed locally. W07 remains in progress for Daemon. No push or remote CI run.

## Implementation

- Moved retry-setting validation rules into `internal/domain/config`; `internal/config` retains compatibility exports for current runtime and legacy callers.
- Added `internal/usecase/settings` and bootstrap wiring over the existing `fileconfig.Store` adapter. HTTP and Native settings entrypoints now use the same typed settings module; `internal/application` retains compatibility facades for existing callers.
- Preserved the intentional write-policy split:
  - Legacy HTTP `PUT /api/settings` validates retry rules, updates the latest config without a revision precondition, and does not create a backup.
  - Native settings draft save validates retry rules and ports, compares the settings revision under the file lock, backs up before the atomic write, and returns the normalized persisted draft.
- Kept settings JSON, missing-file defaults, path selection, and persisted config format unchanged.

## Verification

| Check | Result |
|---|---|
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |

Focused usecase tests verify the legacy no-revision/no-backup contract and that stale Native revisions do not write or back up. Existing application and HTTP tests cover draft normalization, validation, atomic failure, defaults, and settings round-trips.

## Remaining work and limits

- The daemon process implementation/callers still need W07 migration with PID identity and unmanaged-listener refusal unchanged.
- CLI/TUI and some legacy provider handlers still have provider/config mutation paths; they remain for the later entrypoint-convergence work rather than being silently folded into this settings change.
- No live user config, GUI/hardware run, or remote CI was used.
