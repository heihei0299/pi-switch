# Architecture V2 W08d — Revisioned provider path cleanup — 2026-10-13

**Status:** Revisioned/legacy provider wiring slice passed focused local verification. W08 remains in progress. No push, merge, remote CI, or release was performed.

## Implementation

- HTTP's legacy Provider PUT/DELETE now call the provider usecase directly; the unused `application.PutLegacyProvider` and `DeleteLegacyProvider` wrappers were removed.
- Revisioned Native duplicate and active-profile selection now run through provider-usecase transactions. Application retains only the UI-facing validation/redaction/error mapping; the mutation closures were removed.
- Removed the unreferenced `internal/profile.DuplicateInConfig` and `internal/application.updateWithBackup` write paths. Backup/revision semantics, Native validation redaction, and HTTP no-backup behavior remain covered.
- No config shape, API endpoint, or persistence format changed.

## Verification

| Check | Result |
|---|---|
| Focused root tests: `./internal/usecase/providers ./internal/application ./internal/profile ./internal/server ./cmd/pi-switch` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop test -mod=readonly ./...` | PASS |
| Focused root `go vet` for the changed provider, application, profile, server, and CLI packages | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run -mod=readonly ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |
| Production search for removed wrapper/duplicate/write-closure entrypoints | PASS — no matches |

## Remaining W08 work and limits

- Provider model-fetch-and-persist (`fetch-models --channel` and the matching API) still uses the legacy profile path; Desktop still has application workflows that need a final boundary review.
- The final root/Desktop CI-equivalent checks and smoke evidence must be rerun on the final code HEAD after remaining changes. The current full test/vet/secrets/smoke evidence is recorded in the W08 verification work but is not yet a final completion audit.
- The Desktop CI package build was exercised locally for amd64 and arm64, but Debian lifecycle tests are guarded for disposable GitHub runners. Native arm64 runtime, Xvfb window, actual Arch+niri/Wayland, fcitx5, and hardware scaling remain unverified.
