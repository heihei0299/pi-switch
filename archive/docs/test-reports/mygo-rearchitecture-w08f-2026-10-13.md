# Architecture V2 W08f — remove superseded settings application wrappers — 2026-10-13

**Status:** Dead settings-wrapper cleanup passed focused verification. W08 remains in progress. No push, merge, remote CI, or release was performed.

## Implementation

- Removed the unused `application.SaveSettings`, `LoadSettingsDraft`, and `SaveSettingsDraft` wrappers and their settings-specific error mapper. Production Native and HTTP paths already call the settings usecase directly.
- Updated integration tests to exercise `bootstrap.NewSettingsService` instead of keeping obsolete application APIs alive solely for tests. Backup/restore and safe validation behavior remain covered.
- No runtime path, config format, or HTTP/CLI surface changed.

## Verification

| Check | Result |
|---|---|
| `go test -mod=readonly ./internal/application ./internal/usecase/settings ./internal/server` | PASS |
| `CGO_ENABLED=0 go -C desktop test -mod=readonly ./...` | PASS |
| `go vet -mod=readonly ./internal/application ./internal/usecase/settings ./internal/server` | PASS |
| `go run -mod=readonly ./internal/archcheck` | PASS — clean |
| `gofmt -d` and `git diff --check` | PASS — no output |
| Production/test search for removed settings wrappers and mapper | PASS — no references |

## Remaining W08 work and limits

- Continue the production caller audit for other demonstrably unused compatibility orchestration and stale wiring/comments; keep used adapters with user-facing redaction/error semantics.
- Run final root/Desktop tests and vet, Secrets Scan, local Root/Desktop CI-equivalent checks, CLI/API/Proxy smoke, and compatibility audit against the final HEAD.
- No remote GitHub CI was triggered. Arch+niri/Wayland, fcitx5, and hardware scaling remain unverified; Ubuntu/Xvfb is not a substitute.
