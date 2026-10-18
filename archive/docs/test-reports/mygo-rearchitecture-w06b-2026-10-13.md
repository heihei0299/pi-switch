# Architecture V2 W06b — indexed session matching — 2026-10-13

**Status:** Matcher optimization passed locally on top of W06a baseline `5af6851`. W06 remains in progress.

## Implementation

- Added an immutable `conversation.Matcher` index keyed by bare model and sorted by session activity time. Each lookup binary-searches the inclusive ±2 second match window before applying the existing provider, prompt-token, ambiguity, and attribution rules.
- `Stats`, `Conversations`, and `ConversationRequests` build one matcher for each session-scan operation and reuse it across all rows. Non-session-scan sources keep their prior behavior without building an index.
- Added deterministic parity coverage against the existing scan matcher across mixed candidates, plus exact-window-boundary checks. No SQLite schema or attribution rules changed.

## Before/after synthetic benchmark

Same command and 128-candidate synthetic SQLite fixture as W06a; one of the 128 same-model candidates falls in the match window. Each query ran once after DB seeding.

| Rows | Source | Before time / allocated | After time / allocated | Time change | Allocation change |
|---:|---|---:|---:|---:|---:|
| 10k | session scan | 61.5 ms / 170.4 MB | 21.6 ms / 7.1 MB | -64.9% | -95.9% |
| 100k | session scan | 597.3 ms / 1.696 GB | 219.8 ms / 69.7 MB | -63.2% | -95.9% |
| 1M | session scan | 5.976 s / 16.953 GB | 2.156 s / 696.1 MB | -63.9% | -95.9% |
| 1M | source off | 1.812 s / 528.1 MB | 1.790 s / 528.1 MB | -1.2% | effectively unchanged |

Peak RSS for the benchmark process was 204,848 KiB before and 215,364 KiB after; the index adds a small resident snapshot while avoiding repeated per-row candidate-slice allocations. These are synthetic single-process measurements, not production telemetry.

## Verification

| Check | Result |
|---|---|
| `go test -mod=readonly ./internal/conversation ./internal/stats` | PASS |
| `go test -race -mod=readonly ./internal/conversation ./internal/stats` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |

## Remaining W06 work

- Stats SQL still lives directly in `internal/stats.Service`; SQLite query isolation and the shared Native/API analytics usecase remain outstanding. Pagination/index behavior and combined request/session scans need separate measurement and work.
- No production database or real session files were read for these benchmarks.
