# Architecture V2 W09 — architecture contracts — 2026-10-14

**Status:** W09 acceptance passed locally. This ticket changes tests and documentation only; no production Go, configuration, CI, or persisted-data code was changed. No push, merge, live-user-data access, or remote CI run was performed.

## Scope

W09 first audited the current `mygo` implementation and the W01–W08 contract tests. The existing suite already protects the major migrated seams: provider mutation and credential handling, config/backup/locking, Gateway preview/publish, inference routing/conversion/SSE/retry/cancellation/usage, analytics facts and aggregation, packages, settings, daemon identity/lifecycle, CLI/TUI, and Native Desktop flows.

The remaining direct gap was the W08h HTTP full-config replacement boundary. Added `internal/server/config_replacement_contract_test.go` with two external HTTP contract tests:

- `TestPutConfigReplacesWholeDocumentWithoutBackup` proves `PUT /api/config` is replacement rather than merge: stale profiles disappear, the submitted profile/current value survive, omitted settings receive the existing defaults, the file remains mode `0600`, and this legacy endpoint does not create a backup directory.
- `TestPutConfigInvalidReplacementPreservesBytesAndNoBackup` proves a rejected capability combination returns `400` before persistence and leaves the existing config bytes and backup state unchanged.

The report itself is the only new documentation file: `docs/test-reports/mygo-rearchitecture-w09-2026-10-14.md`.

## Verification

All stateful checks used isolated temporary paths under `/tmp`; no real user config, database, credentials, models file, or session directory was used.

| Check | Result |
|---|---|
| Focused W09 HTTP contract tests | PASS — 2/2 |
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` | PASS |
| `CGO_ENABLED=0 go -C desktop test -mod=readonly ./...` | PASS |
| `CGO_ENABLED=0 go -C desktop vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run -mod=readonly ./internal/archcheck` | PASS — `architecture dependencies: clean` |
| `git diff --check` for W09 test changes | PASS |

## Acceptance and remaining risk

W09 is accepted locally: the key post-refactor behavior matrix has regression coverage, the added contract tests pass, and both Go modules plus the architecture checker pass without production changes.

Remaining risk is verification scope, not a known W09 failure: GitHub Actions/CI runner differences, native arm64 execution, and Arch+niri/Wayland/IME/hardware-scaling GUI gates were not run here. Those are environment/release gates and are not claimed as W09-local evidence.
