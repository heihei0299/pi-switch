# Architecture V2 W08c — Legacy provider mutations through usecase — 2026-10-13

**Status:** Provider mutation slice passed focused local verification. W08 remains in progress. No push, merge, remote CI, or release was performed.

## Implementation

- CLI and HTTP create/duplicate/expose operations, HTTP model-pool and spoof updates, and the CLI/TUI selection/deletion flows now use typed `internal/usecase/providers` operations.
- The usecase preserves the legacy no-backup contract, HTTP rename/revision differences, idempotent CLI delete, missing-profile error priority, channel materialization, model/exposure validation, and spoof clearing/validation. Provider duplicate validation keeps the old capability-only gate, including repairability of legacy shape/retry issues.
- Moved channel materialization into `internal/domain/provider`; the remaining `internal/profile` channel entrypoint delegates to it. Removed the now-unreferenced `CreateProfile`, `DuplicateProfile`, and `SetExposedModels` persistence paths and moved their integration/concurrency coverage to the provider-usecase suite.
- No config fields, Gateway JSON, or persistence format changed.

## Verification

| Check | Result |
|---|---|
| Focused root tests: `./cmd/pi-switch ./internal/server ./internal/tui ./internal/usecase/providers ./internal/profile ./internal/domain/provider ./internal/adapters/outbound/profilevalidation ./internal/application` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -race -mod=readonly ./internal/usecase/providers -run '^TestConcurrentLegacyCreatesPreserveProfiles$' -count=3` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop test -mod=readonly ./...` | PASS |
| Focused root `go vet` for the changed command, server, TUI, provider usecase, profile, domain, adapter, and application packages | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run -mod=readonly ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |
| Production search for CLI/HTTP calls to removed profile CRUD/exposure helpers or direct profile config mutation closures | PASS — no matches |

## Remaining W08 work and limits

- `fetch-models --channel` and `/api/profiles/:name/fetch-models?channel=` still use the legacy profile fetch-and-persist path. Native/Desktop and other remaining `internal/application` consumers still need a final boundary audit; remove only wrappers with no remaining consumers.
- Final root/Desktop test and vet suites, Secrets Scan, local Root/Desktop CI-equivalent checks, and CLI/API/Proxy smoke still need final-HEAD evidence.
- Remote GitHub CI was not triggered. Arch+niri/Wayland, fcitx5, and hardware scaling remain unverified; Ubuntu/Xvfb is not a substitute.
