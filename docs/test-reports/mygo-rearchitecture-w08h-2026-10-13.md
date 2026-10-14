# Architecture V2 W08h — move full-config replacement out of HTTP handler — 2026-10-13

**Status:** HTTP full-config replacement slice passed focused verification. W08 remains in progress. No push, merge, remote CI, or release was performed.

## Implementation

- Moved the legacy `/api/config` whole-config validation and save operation into `application.ReplaceConfig`, which uses the existing file-config adapter.
- The HTTP handler now only reads/parses the request, invokes the operation, and preserves the existing 400 validation and 500 persistence envelopes.
- Removed the stale server comment claiming management writes use `config.UpdateAtPath` directly. No endpoint, config shape, or backup behavior changed.

## Verification

| Check | Result |
|---|---|
| `go test -mod=readonly ./internal/server ./internal/application` | PASS |
| `go vet -mod=readonly ./internal/server ./internal/application` | PASS |
| `go run -mod=readonly ./internal/archcheck` | PASS — clean |
| `gofmt -l` and `git diff --check` | PASS — no output |
| Production server search for direct config Save/Update calls | PASS — no matches |
| Existing `/api/config` validation matrix and blocked-write status tests | PASS as part of server suite |

## Remaining W08 work and limits

- Finish the application/Desktop caller and compatibility-facade audit; retain only surfaces with demonstrated consumers or deliberate contract coverage.
- Run final root/Desktop tests and vet, Secrets Scan, local Root/Desktop CI-equivalent checks, CLI/API/Proxy smoke, and compatibility audit against the final HEAD.
- No remote GitHub CI was triggered. Arch+niri/Wayland, fcitx5, and hardware scaling remain unverified; Ubuntu/Xvfb is not a substitute.
