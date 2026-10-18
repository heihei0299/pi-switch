# Architecture V2 W07c — Proxy process-control adapter — 2026-10-13

**Status:** W07 local acceptance passed. W08 remains. No push or remote CI run.

## Implementation

- Moved the existing process identity, pidfd signaling, PID/operation-lock, health, start, stop, and status implementation with its tests from `internal/daemon` to `internal/adapters/outbound/daemon`; process-lifecycle logic was not rewritten.
- Added `internal/usecase/proxycontrol` with typed targets/results and Start/Stop/Status/Restart operations. Bootstrap wires the outbound adapter; HTTP, CLI, and Native Proxy controls now call the usecase.
- Preserved typed port-in-use classification. Restart does not start a new process if a stop is refused or fails. The Native custom executable path remains supported.

## Verification

| Check | Result |
|---|---|
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |

The moved daemon suite covers PID reuse/identity mismatch, unmanaged listener refusal, health ownership, stale runtime state, operation locking, idempotent stop, and safe signal/force-kill paths. New usecase tests verify restart ordering and that a failed stop never starts another process.

## Remaining limits

- No live Proxy process was started/stopped, and no manual GUI or remote-CI run was performed; daemon behavior is verified by isolated tests only.
- W08 remains for residual provider/config entrypoint convergence, final compatibility audit, and its specified smoke/CI evidence.
