# Architecture V2 W06e — reuse same-window analytics aggregation — 2026-10-13

**Status:** Passed locally. W06 acceptance checks are complete; W07 remains. No push or remote CI run.

## Implementation

- Added `Service.StatsAndConversations`. When request and conversation windows are identical (including both all-time), it pages the conversation summaries already aggregated by `Stats` instead of issuing a second history scan. Different windows still use independent reads.
- Native Stats now uses this shared operation. HTTP handlers and response contracts are unchanged.
- Added counting-repository tests proving one fact scan for matching windows and two for distinct windows, plus result/pagination checks.

## Same-window before/after benchmark

Synthetic SQLite history; session-scan source with 128 candidates (one in the match window); one measured iteration after seeding:

```text
GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./internal/usecase/analytics -run '^$' -bench '^BenchmarkStatsAndConversations$' -benchtime=1x -benchmem -count=1
```

| Rows | Separate `Stats` + `Conversations` | Shared-window operation | Time change | Allocated/op change |
|---:|---:|---:|---:|---:|
| 10k | 43.34 ms / 14.06 MB | 21.63 ms / 7.07 MB | -50.1% | -49.7% |
| 100k | 424.13 ms / 139.34 MB | 218.06 ms / 69.71 MB | -48.6% | -50.0% |
| 1M | 4.336 s / 1.392 GB | 2.164 s / 696.1 MB | -50.1% | -50.0% |

This is a synthetic local comparison, not production telemetry. Distinct windows correctly retain two scans; no broad union scan was introduced.

## Verification

| Check | Result |
|---|---|
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |

Together with W06a–W06d, full-history matcher benchmarks, selective-window index benchmarks, null/unknown/cost/cache/conversation contracts, and repository pagination/window tests are recorded. No production DB or real session files were read.

## Remaining risk

The additive timestamp expression index's one-time build/cold-open cost, disk growth, and write overhead were not measured on a production-sized database. The index does not rewrite request rows. Real-user-data and GUI/hardware checks were not run.
