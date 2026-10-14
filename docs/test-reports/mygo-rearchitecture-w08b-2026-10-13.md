# Architecture V2 W08b — CLI/TUI profile selection and deletion — 2026-10-13

**Status:** CLI/TUI mutation slice passed focused local verification. W08 remains in progress. No push, merge, remote CI, or release was performed.

## Implementation

- Added typed provider-usecase operations for no-backup profile selection and the CLI's idempotent delete contract.
- CLI `provider use/delete` and TUI profile selection now call the provider usecase instead of embedding config mutation closures.
- Preserved missing-profile selection errors, current-profile clearing, no-backup behavior, and successful `provider delete <missing-name>` behavior. Added usecase and CLI contract coverage.
- No config fields or persistence format changed.

## Verification

| Check | Result |
|---|---|
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./cmd/pi-switch ./internal/tui ./internal/usecase/providers ./internal/adapters/outbound/fileconfig` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./cmd/pi-switch ./internal/tui ./internal/usecase/providers ./internal/adapters/outbound/fileconfig` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run -mod=readonly ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |
| Production search for direct `config.UpdateAtPath` in CLI/TUI | PASS — no matches |

## Remaining W08 work and limits

- Review and converge remaining provider/profile HTTP and Native paths without changing their legacy response, validation, revision, backup, or no-write semantics; remove only facades proven unused.
- Run final root/Desktop test and vet suites, Secrets Scan, local Root/Desktop CI-equivalent checks, and CLI/API/Proxy smoke with evidence tied to final HEAD.
- Remote GitHub CI and actual Arch+niri/Wayland, fcitx5, and hardware scaling remain unverified.
