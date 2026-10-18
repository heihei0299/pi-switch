# Architecture V2 W12 — compatibility-layer convergence — 2026-10-14

**Status:** W12 complete. Only W12 changes are included in the W12 commit; no push was performed.

## Scope and inventory

The compatibility surface was located with `rg` before editing:

- `internal/gateway/` is the old Gateway compatibility facade. Its implementation already delegates business rules to `internal/domain/gateway` and I/O to `internal/bootstrap`/the outbound Gateway adapter.
- The only non-facade Go callers were test fixtures:
  - `desktop/native_complex_test.go`
  - `internal/tui/tui_daemon_test.go`
- No production CLI, TUI, HTTP, Native, use-case, or adapter path imported `internal/gateway`.
- `internal/archcheck/main_test.go` contains a synthetic forbidden-import fixture only; it is not a runtime caller.

## Changes

- Kept `internal/gateway` and its exported forwarding API to preserve the old internal interface.
- Removed test-only constants, stale-error alias, and provider/map helpers from the production facade; compatibility tests now name the canonical domain symbols directly.
- Moved the two direct test callers to the canonical domain Gateway package and the bootstrap Gateway service.
- Added `internal/gateway/compatibility_test.go`, an external-package compatibility test proving the legacy facade's generated proposal and fixed-provider list match the canonical domain implementation.
- No HTTP route, CLI command, config/SQLite schema, Gateway JSON format, or runtime behavior was changed.

## Verification

| Check | Result |
|---|---|
| `go test -mod=readonly ./internal/gateway ./internal/tui` | PASS |
| `CGO_ENABLED=0 go -C desktop test -mod=readonly ./...` | PASS |
| `go test -mod=readonly ./...` | PASS |
| `go vet -mod=readonly ./...` | PASS |
| `CGO_ENABLED=0 go -C desktop vet -mod=readonly ./...` | PASS |
| `go run -mod=readonly ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |
| `rg` audit for old compatibility-package callers | PASS — only the intentional compatibility test imports `internal/gateway` |

## Remaining risk

The legacy facade remains intentionally because the old interface is still a compatibility contract. It is now a forwarding surface rather than a second owner of Gateway rules. Remote CI, native arm64/QEMU, and Arch+niri/Wayland/fcitx5/hardware-scaling validation were not run for W12.
