# Architecture V2 W06d — selective history window index — 2026-10-13

**Status:** Focused tests and synthetic benchmarks passed locally. W06 remains in progress; no push or remote CI run.

## Implementation

- Added `idx_requests_ts_epoch_ms`, an expression index matching the existing millisecond timestamp conversion used by the SQLite analytics adapter. `store.Open` creates it additively for both new and existing databases; request rows and response DTOs are unchanged.
- Added a migration test for index creation and an adapter query-plan test that verifies timestamp-window queries use it.
- Added a selective-window history benchmark at 10k, 100k, and 1M rows. The synthetic fixture distributes rows across 100 days and selects one day (~1% of rows).

## Before/after

Command (one measured iteration after fixture seeding):

```text
GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./internal/usecase/analytics -run '^$' -bench '^BenchmarkStatsHistorySelectiveWindow$' -benchtime=1x -benchmem -count=1
```

| Rows | No index time/op | Indexed time/op | Change | Allocated/op before → after |
|---:|---:|---:|---:|---:|
| 10k | 9.04 ms | 1.07 ms | -88.2% | 124.6 KB → 124.4 KB |
| 100k | 167.87 ms | 4.34 ms | -97.4% | 600.2 KB → 600.2 KB |
| 1M | 1.896 s | 32.20 ms | -98.3% | 5.35 MB → 5.35 MB |

The allocation totals are nearly unchanged; the gain is avoiding full-history timestamp-expression evaluation for selective windows. The query plan uses the expression index for the range and a temporary B-tree for the existing `ORDER BY id DESC`. These are synthetic local measurements, not production data.

## Verification

| Check | Result |
|---|---|
| `go test -mod=readonly ./internal/store ./internal/adapters/outbound/sqlanalytics` | PASS |
| `TestRepositoryWindowQueryUsesTimestampIndex` (modernc SQLite planner) | PASS |
| Selective-window benchmark, before and after index | PASS; results above |

## Limitations

- Full-history scans do not benefit from this index; W06a/W06b retain those 10k/100k/1M measurements.
- One-time index build/cold-start cost, additional database size, and per-request write overhead were not measured. The index is additive and does not rewrite request rows, but large existing histories may take longer to open once while SQLite builds it.
- Native Stats and Conversations can still traverse the same window separately; W06 remains in progress pending that repeated-scan reduction and final full checks.
