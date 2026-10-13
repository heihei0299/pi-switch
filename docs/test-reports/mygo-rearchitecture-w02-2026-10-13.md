# Architecture V2 W02 — Config / Backup / Credential IO — 2026-10-13

**Status:** W02 acceptance passed locally. Previous ticket: W01 `02c2dc4`. No push, live-user-data test, or manual GUI smoke was run.

## Implementation

- `internal/domain/config` now owns the persisted config types, settings JSON compatibility, parse behavior, and save-time migration. `internal/config` keeps type/function compatibility and delegates path resolution and file transactions to `fileconfig`.
- `internal/adapters/outbound/fileconfig` is the only production owner of config and backup filesystem IO: cross-process lock, atomic 0600 writes, update/replace, backup creation/list/read, and config revision. The provider storage port still composes through this adapter; lock waiting now returns promptly on context cancellation.
- Provider credential masking/restoration moved into `internal/usecase/providers`. The application facade still maps malformed retained markers to its existing validation error.
- The architecture checker no longer permits `fileconfig` to import `internal/config`; the SPEC now records the W02 compatibility boundary. The separate W01 shared-profile-validator bridge remains scoped to `outbound/profilevalidation`.

## Contract evidence

The existing config/application tests passed after the move and continue to cover config shape/migration, cross-process update locking, atomic writes and 0600 permissions, mutation/backup failures without config writes, exact pre-write/recovery bytes, backup symlink/private-directory rejection, and application-level credential redaction. New tests verify context cancellation while another process holds the lock (including unchanged config bytes) and verify masked provider JSON contains none of the original URL/key/header secrets before exact in-memory restoration.

## Verification

| Check | Result |
|---|---|
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (`desktop/`) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (`desktop/`) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -race -mod=readonly ./internal/config ./internal/adapters/outbound/fileconfig ./internal/usecase/providers ./internal/application` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |

## Remaining limits

- Existing entrypoints retain the `internal/config` compatibility functions; they now delegate to the same adapter. Removing those legacy calls remains part of later migration work.
- No Windows cross-build, remote CI, live config, or GUI smoke was run for this ticket.
