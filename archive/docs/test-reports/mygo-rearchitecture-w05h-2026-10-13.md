# Architecture V2 W05h — stream outbound execution — 2026-10-13

**Status:** The W05 route/plan/non-stream/stream execution slices are implemented locally on top of W05g `1fcbb4f`. Adapter-only HTTP mapping and request persistence remain in `internal/server`. No push or remote CI run.

## Implementation

- Added `inference.ExecuteStream` to build/send the upstream request through the server adapter, preserve the 64 KiB upstream-error body cap, classify transport cancellation, and invoke `RelayStream` for successful responses.
- `RelayStream` now sets the upstream status after preparing response headers, so the handler no longer coordinates stream status before relaying.
- Reduced `handleStream` to building the transport request and mapping the returned outcome to Gin/logging. Shared the outbound-builder adapter between stream and non-stream execution.
- Removed the now-unused server URL helper and stream-error limit from the retry-scheduler file. No failover/retry scheduling was enabled.
- Added usecase tests for upstream status propagation, bounded error bodies, cancellation, and request-build failures. Existing HTTP tests cover stream conversion, 429 passthrough, and client cancellation.

## Verification

| Check | Result |
|---|---|
| Targeted inference and server stream/cancellation/429 tests | PASS |
| Targeted `go test -race` (inference relay/execution and server stream paths) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |

`internal/server/channel_route_test.go::TestProxyRoute_ChannelPinnedID` remains skipped in this checkout (`failover removed, test skipped`) and is not treated as evidence. The request-preparation tests cover unique exposed-channel selection; the skipped HTTP route test remains an unverified case.

## Remaining plan work

- W05 extraction is complete locally, but the skipped channel-route test and absence of remote CI/live upstream validation remain verification limits.
- W06–W08, including final CI/secrets/smoke gates, are still outstanding. No live upstream, GUI, or real user data was used for this slice.
