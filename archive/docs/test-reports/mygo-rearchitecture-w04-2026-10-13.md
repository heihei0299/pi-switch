# Architecture V2 W04 — Desktop interaction and state controllers — 2026-10-13

**Status:** W04 acceptance passed locally on top of W03 `7902415`. No push or remote CI run.

## Implementation

- `desktopShell` now keeps window/tray lifecycle, navigation, exit state, and the shared task/UI-thread coordinator. Overview, Proxy, Providers, Settings, Backups, Doctor, Gateway, JSON, Stats/Sessions, and Packages state and page operations live with their page controllers.
- Moved the remaining Gateway, Stats/Sessions, and Packages task orchestration out of `shell.go`; settings, backup, diagnostic, and provider tasks also remain on their page controllers. Navigation uses page callbacks, and cross-page reads use explicit Overview accessors.
- The shared task registry still guards every async result by task token and rejects results after finish/quit. The quit handler cancels UI tasks but does not stop Proxy; stopping Proxy remains an explicit user action.
- Existing Native UI tester coverage continues through the desktop test suite. No framework or event bus was added.

## Verification

| Check | Result |
|---|---|
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (`desktop/`, including Native UI tester) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -race -mod=readonly ./...` (`desktop/`) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (`desktop/`) | PASS |
| `git diff --check -- desktop` | PASS |

## Remaining limits

- No manual GUI smoke or remote CI was run for this ticket; automated Native UI tester coverage passed.
- No live user config, request history, or running Proxy process was used. Proxy quit behavior is verified by the quit path (task cancellation only) and the absence of an automatic stop call, not by stopping a live process.
