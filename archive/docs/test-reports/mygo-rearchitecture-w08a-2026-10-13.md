# Architecture V2 W08a — Gateway entrypoint convergence — 2026-10-13

**Status:** Gateway entrypoint slice passed focused local verification. W08 remains in progress. No push, merge, remote CI, or release was performed.

## Implementation

- CLI, TUI, Native Desktop, and HTTP Gateway handlers now read and publish through `bootstrap.NewGatewayService`; Gateway rules use `internal/domain/gateway` directly.
- Config reads from the shared server path and executable entrypoints now pass through `bootstrap.LoadConfig` and the existing file-config adapter. HTTP error status/envelopes remain unchanged.
- Gateway health's `has_models_file` check now goes through the Gateway store/usecase boundary. Its existing file-present/file-missing contract remains covered by the HTTP health tests and adapter tests.
- Removed the unused `internal/application/gateway.go` compatibility wrapper. Kept `internal/gateway` as a compatibility façade because its package-level contract suite still exercises the legacy surface; no production entrypoint imports it.
- Updated Desktop CI's daemon test path to the daemon's new outbound-adapter package. Gateway JSON and config persistence formats were not changed.

## Verification

| Check | Result |
|---|---|
| Focused root tests: `./cmd/pi-switch ./internal/tui ./internal/server ./internal/usecase/gateway ./internal/adapters/outbound/filegateway ./internal/application` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop test -mod=readonly ./...` | PASS |
| Focused root `go vet` for the changed command, TUI, server, Gateway, bootstrap, and application packages | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run -mod=readonly ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |
| Production-path search for old `internal/gateway`, Gateway application wrappers, and `config.LoadConfigAtPath` under CLI/TUI/Desktop | PASS — no matches |

## Remaining W08 work and limits

- Converge the remaining provider/config mutation paths and remove any other demonstrably unused orchestration/facades without changing their legacy contracts.
- Run final root and Desktop test/vet suites, Secrets Scan, local Root/Desktop CI-equivalent checks, and CLI/API/Proxy smoke with evidence tied to final HEAD.
- No remote GitHub CI was triggered. Actual Arch+niri/Wayland, fcitx5, and hardware scaling gates remain unverified; the available Ubuntu/Xvfb run cannot substitute for them.
