# Architecture V2 W00 baseline — 2026-10-13

**Status:** W00 acceptance passed locally. This work adds architecture checks and behavioral tests only; no production behavior or persisted format changed. No remote CI, push, merge, or live-data test was run.

## Source baseline and comparison

- Plan/spec baseline: `51530102c94dfd5412df25eb0e612c73a5d668f1`.
- The starting `HEAD` was `5bce3e8`; its only changes after the baseline were the two Architecture V2 planning documents. No production Go source differed from the stated baseline.
- `main` and `origin/main` were `18f877e60ef62e294bea8ad65bfebc5838ee96c1`, the merge base of the plan baseline and `mygo`. The root-module source delta is large (238 files); the current `mygo` branch also has the standalone Desktop module and has retired the old embedded WebUI.
- A clean archive of `main` was tested with isolated `PI_SWITCH_CONFIG`, `PI_SWITCH_DB`, `PI_SWITCH_MODELS`, `PI_SWITCH_CONFIG_DIR`, and `PI_AGENT_SESSIONS`. `go test -mod=readonly ./...` did **not** pass: the `cmd/pi-switch`, `internal/server`, and `webui` packages cannot compile because `webui/embed.go` requires an absent `webui/dist`; `internal/daemon` separately failed `TestStart_ReturnsQuicklyWhenItsChildDiesOnStartup` and `TestTuiDaemon_S5_StartEADDRINUSE` on timeout assertions. Other listed packages passed. No npm dependencies were installed and no generated assets were copied into the archive, so this is recorded as an incomplete/failed `main` comparison, not as a passing baseline or a product regression.
- `main` has no separate `desktop/go.mod`; Desktop-module comparison is only available on `mygo`.

## Frozen Provider contracts

| Surface | Authoritative checks and assertions |
|---|---|
| HTTP `PUT` / `DELETE` | New `internal/server/profile_mutation_contract_test.go`: success status/body (`backup:null`), exact serialized bytes against the legacy `config.UpdateAtPath` mutation, no backup directory, legacy rename/current behavior (`PUT renameFrom` leaves current unchanged), `DELETE` clears current, missing delete is 404 with unchanged bytes, and invalid-profile validation wins before config I/O. |
| Native provider save/delete | `internal/application/management_test.go`: successful rename updates current; save and delete each preserve exact pre-write bytes in one mode-0600 backup; stale save/delete revisions are conflicts with unchanged config and no backups. Existing `desktop/native_standard_test.go` covers cancel/no-write and Native interaction behavior. |
| CLI | New `cmd/pi-switch/provider_mutation_contract_test.go`: `use` only changes current; deleting current clears it and preserves other profiles; missing `use` changes no bytes; these CLI operations create no backups. Existing add/duplicate/expose/fetch tests cover validation, errors, and no-write failures; `provider_duplicate_test.go` now explicitly fixes CLI duplicate's no-backup behavior. CLI has no rename or optimistic-revision operation. |
| Shared concurrent writes | New `TestHTTPAndNativeProviderMutationsDoNotLoseConcurrentUpdates`; existing `internal/config/update_at_path_test.go:TestUpdateAtPathAcrossProcesses` exercises eight concurrent child processes, and `internal/profile/concurrent_create_test.go` checks concurrent provider creation. |

The intentional HTTP/Native/CLI backup, revision, and rename differences remain explicit; W00 does not unify them.

## Other frozen contracts already covered

- **Gateway:** `internal/gateway/gateway_test.go` covers stale-plan refusal without overwriting concurrent bytes, third-party provider preservation (including exact entry and mode 0600), idempotent publish, and invalid-plan zero-write. Server and Native tests cover read-only preview, explicit publish, conflict/no-op refusal, and config/model file separation.
- **Proxy process identity:** `internal/daemon/identity_test.go` covers legacy PID refusal, unmanaged healthy listener refusal without termination, health served by a different process, PID reuse, changed identity, and managed-process stop. Operation-lock tests cover concurrent mutation and recovery.
- **Stats and usage:** `internal/usage/usage_test.go`, `internal/stats/unknown_usage_test.go`, `internal/stats/service_test.go`, and `internal/server/unknown_usage_test.go` fix unknown-vs-explicit-zero token facts, cached/reasoning details, unknown cache-rate behavior, exports, and immutable request facts.

## Dependency checker

- Added `internal/archcheck`, using only the Go standard library and `go list std`.
- `go run ./internal/archcheck` passes on the legacy/current tree, where the V2 package trees do not yet exist.
- `go test ./internal/archcheck -run 'TestCheckArchitectureDependencies$' -count=1 -v` passes clean fixtures and deliberately injected forbidden imports: domain/usecase third-party imports, usecase-to-adapter imports, inbound/outbound cross-imports, misplaced adapter code, and root-module imports of the Desktop module from bootstrap, command, and WebUI packages.
- Added the checker to both root CI and `mygo` Desktop CI. The exact import matrix and production-source-only scope are in `SPEC-ARCHITECTURE-V2.md`.

## Verification on `mygo`

All commands used isolated temporary config/database/models/session paths, `GOTOOLCHAIN=local`, and `GOPROXY=off`.

| Command | Result |
|---|---|
| `go test -mod=readonly ./...` (root module) | PASS |
| `go vet -mod=readonly ./...` (root module) | PASS |
| `go test -mod=readonly ./...` (`desktop` module) | PASS |
| `go vet -mod=readonly ./...` (`desktop` module) | PASS |
| `go run ./internal/archcheck` | PASS — `architecture dependencies: clean` |
| Dependency-checker negative/clean fixture tests | PASS |

The existing dirty `.gitignore`, `.idea`, test-report, `AGENTS.md`, `.commandcode`, `tmp`, and `webui` files were not changed or included in this work. No package install or GUI retest was needed for W00.
