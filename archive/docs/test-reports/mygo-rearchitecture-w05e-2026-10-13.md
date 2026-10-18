# Architecture V2 W05e — request preparation extraction — 2026-10-13

**Status:** W05e passed locally on top of W05d `58ad645`. W05 as a whole is still incomplete. No push or remote CI run.

## Implementation

- Added `inference.PrepareRequest` to own route resolution, channel narrowing, upstream/model selection, token-limit clamping, protocol planning, request transformation, and stream-converter validation. The HTTP handler retains body intake/defaulting, slash-ID validation, conversation lookup, HTTP response mapping, and outbound dispatch.
- Kept the manual non-stream 400 retry policy and dispatch in the HTTP handler; moved only construction of its transformed, minimum-token retry body onto `PreparedRequest`.
- Put token-limit rules under `internal/domain/limit`; the former server-only clamp tests were replaced by preparation-level tests. Added provider profile helpers for selecting the primary upstream and narrowing to a channel, reused by the existing dormant retry path.
- Added tests for global route selection, selecting the channel exposing a model, Responses-to-Chat request transformation and clamping, stream preparation/unsupported conversion, retry-body construction, encrypted-content clamping, and absent/unknown model metadata.
- Did not change outbound dispatch, retry conditions, response conversion, usage/cost extraction, or request logging behavior.

## Verification

| Check | Result |
|---|---|
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run ./internal/archcheck` | PASS — clean |
| Targeted race tests for preparation, stream relay, retry, protocol, cancellation, and 429 handling | PASS |
| `git diff --check` | PASS |

`internal/server/channel_route_test.go::TestProxyRoute_ChannelPinnedID` is skipped in this checkout (`failover removed, test skipped`); it is not treated as passing evidence. The new usecase test verifies selection of the channel that exposes a bare model, but does not substitute for that skipped HTTP test.

## Remaining limits

- W05 remains incomplete: non-stream outbound coordination, response conversion/result handling, usage/cost extraction, and persistence/log coordination still live in `internal/server`; only SSE relay and request preparation have moved into the inference usecase.
- W06–W08 and final CI/secrets/smoke gates remain outstanding. No live upstream, GUI, or real user data was used for this slice.
