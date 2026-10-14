# Architecture V2 W07a — typed Packages usecase — 2026-10-13

**Status:** Package slice passed locally. W07 remains in progress for Settings and Daemon. No push or remote CI run.

## Implementation

- Added `internal/usecase/packages` with typed add commands, package DTOs, import results, validation, and CRUD/import orchestration.
- Moved package-table SQL and the Pi package discovery implementation behind `sqlpackages` and `piagentpackages` outbound adapters. Bootstrap is the single composition point.
- HTTP handlers, CLI, and Native Packages now use the same typed usecase. JSON keys and values, not-found mapping, soft-delete/toggle behavior, package import transaction/upsert/stale-row behavior, and the existing SQLite table/migrations are preserved.
- Kept `internal/packages` as a compatibility facade for remaining internal callers; it delegates to the same usecase. Reads of a missing package DB return an empty list without creating a file, preserving Native's existing no-create behavior.
- Added coverage for legacy package-table migration, import/delete/toggle behavior, missing Pi settings (including no DB creation), and CLI/Native/API flows. No persisted package rows or table format were changed.

## Verification

| Check | Result |
|---|---|
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |

## Remaining W07 work

- Settings writes still need all active inbound callers routed through the application/usecase while preserving legacy HTTP no-revision behavior and Native path/revision/backup semantics.
- Daemon remains in `internal/daemon`; move its use behind the outbound adapter/usecase while retaining PID identity, unmanaged-listener refusal, restart/stop idempotency, and explicit-only Proxy stop.
