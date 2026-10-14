# Architecture V2 W08g — converge read-only provider operations — 2026-10-13

**Status:** Read-only provider entrypoint slice passed focused verification. W08 remains in progress. No push, merge, remote CI, or release was performed.

## Implementation

- Routed CLI and HTTP provider probes through the shared application operation; Native draft probing reuses the same primitive.
- Routed CLI model-ID listing and HTTP's legacy read-only fetch/enrichment through shared application operations. The no-channel CLI remains read-only; the no-channel HTTP route keeps its empty-base-URL response and does not persist.
- Removed direct `internal/profile` network/enrichment calls from `cmd/pi-switch` and HTTP profile handlers. Existing upstream implementations remain behind the application workflow; no endpoint, output shape, or config format changed.

## Verification

| Check | Result |
|---|---|
| Focused root tests: `./cmd/pi-switch ./internal/server ./internal/application ./internal/profile` | PASS |
| Focused root `go vet` for the same packages | PASS |
| `go run -mod=readonly ./internal/archcheck` | PASS — clean |
| `gofmt -l` and `git diff --check` | PASS — no output |
| Production search for direct profile test/fetch/enrichment calls in CLI and HTTP handlers | PASS — no matches |
| Existing CLI/API fetch, upstream-header, error, and read-only config tests | PASS as part of focused suites |

## Remaining W08 work and limits

- Review remaining direct HTTP config replacement/persistence and stale comments, then complete the Desktop/application caller audit without moving user-facing redaction or draft semantics.
- Run final root/Desktop tests and vet, Secrets Scan, local Root/Desktop CI-equivalent checks, CLI/API/Proxy smoke, and compatibility audit against the final HEAD.
- No remote GitHub CI was triggered. Arch+niri/Wayland, fcitx5, and hardware scaling remain unverified; Ubuntu/Xvfb is not a substitute.
