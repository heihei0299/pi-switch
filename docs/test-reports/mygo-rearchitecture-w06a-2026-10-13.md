# Architecture V2 W06a — analytics baseline — 2026-10-13

**Status:** Synthetic SQLite baseline recorded before W06 performance changes. No production data was read or changed.

## Benchmark setup

Added `BenchmarkStatsHistory` for `Service.Stats` over temporary SQLite databases with 10k, 100k, and 1M request rows. The session-scan case uses 128 synthetic sessions for one model, spaced one minute apart; requests share one timestamp, so one session is in the matcher window. Each benchmark invocation runs once (`-benchtime=1x`); database seeding is outside the timed section. The database uses the repository's normal `store.Open` schema.

Command:

```text
go test -mod=readonly ./internal/stats -run '^$' -bench '^BenchmarkStatsHistory$' -benchtime=1x -benchmem -count=1
```

| Rows | Source | Time/op | Allocated/op | Allocs/op |
|---:|---|---:|---:|---:|
| 10k | off | 19.0 ms | 5.35 MB | 371,877 |
| 10k | session scan (128 candidates) | 61.5 ms | 170.4 MB | 402,076 |
| 100k | off | 183.8 ms | 52.9 MB | 3,701,868 |
| 100k | session scan (128 candidates) | 597.3 ms | 1.70 GB | 4,002,079 |
| 1M | off | 1.813 s | 528.1 MB | 37,001,868 |
| 1M | session scan (128 candidates) | 5.976 s | 16.95 GB | 40,002,176 |

The complete benchmark process took 22.3 s and peaked at 204,848 KiB RSS. The 1M-row session case allocates substantially more than its peak resident memory because short-lived matcher allocations are reclaimed during the run.

## Findings and limits

- Session matching increases 1M-row `Stats` time from 1.81 s to 5.98 s and allocations from 528 MB to 16.95 GB in this deliberately same-model synthetic workload.
- This is a baseline, not a production-data prediction. It measures the current Go matcher plus SQL scan/aggregation; it does not include GUI/API serialization or disk latency under concurrent writes.
- The next W06 slice will index session candidates for repeated matching, then rerun the same benchmark and the nullable-usage/conversation contract tests.
