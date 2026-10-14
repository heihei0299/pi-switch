# Architecture V2 W11 — Analytics / SQLite cancellation and benchmark — 2026-10-14

**Status:** W11 local acceptance passed. No push was performed.

## Scope

- Added context-aware analytics query methods while keeping the existing methods as `context.Background()` compatibility delegates:
  - `SummaryContext`
  - `ExportRequestsContext`
  - `StatsContext`
  - `StatsAndConversationsContext`
  - `ConversationsContext`
  - `ConversationRequestsContext`
- Changed the Analytics repository port and SQLite adapter to receive `context.Context`.
- SQLite uses `QueryContext` and checks cancellation while iterating rows; the use case also checks cancellation during aggregation and before the paged read.
- Native Desktop Stats refresh and request-history export now pass the task context through the analytics query path. Existing helper entry points remain compatible for tests and non-cancellable callers.
- Updated the existing synthetic benchmarks to exercise the context-aware path. No schema, DTO, aggregation, pagination, nullable-value, cost, cache-rate, or conversation-matching rule changed.

## Result consistency

`TestContextQueriesMatchLegacyResults` compares the existing `Stats` result with `StatsContext(context.Background(), ...)` over the same fixture. The result is identical. Existing analytics and SQLite adapter regression tests continue to cover nullable facts, timestamp windows, ordering, pagination, cost/cache/conversation aggregation, and export aliases.

## Cancellation checks

| Check | Result |
|---|---|
| Usecase cancelled `StatsContext` and `SummaryContext` | PASS; returns `context.Canceled` before reading facts |
| SQLite adapter cancelled `ForEachFact` and `PageFacts` | PASS; returns `context.Canceled` before query execution |
| Native Desktop Stats worker | PASS by code path; task context is passed to `StatsAndConversationsContext` and late results are still rejected by the existing task token/window checks |

## Synthetic benchmarks

Command:

```text
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -mod=readonly ./internal/usecase/analytics -run '^$' -bench '^BenchmarkStats(History|HistorySelectiveWindow|AndConversations)$' -benchtime=1x -benchmem -count=1
```

Environment: Linux amd64, Intel Core Ultra 5 225H. Each sub-benchmark ran one measured iteration; fixture insertion was outside the timed section. The complete benchmark command passed in 71.624s.

| Benchmark | 10k | 100k | 1M |
|---|---:|---:|---:|
| `StatsHistory/source_off` | 18.97 ms | 182.01 ms | 1.787 s |
| `StatsHistory/session_scan_128` | 22.07 ms | 221.51 ms | 2.213 s |
| `StatsHistorySelectiveWindow` | 1.05 ms | 4.20 ms | 31.91 ms |
| `StatsAndConversations/separate_scans` | 43.21 ms | 425.43 ms | 4.221 s |
| `StatsAndConversations/shared_window` | 21.62 ms | 215.50 ms | 2.154 s |

The 1M-row allocation results were also reported successfully: approximately 528 MB for history `source_off`, 696 MB for session attribution/shared-window, and 1.392 GB for separate scans. These are synthetic local measurements, not production telemetry or optimization claims.

## Verification

| Check | Result |
|---|---|
| `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go vet -mod=readonly ./...` | PASS |
| `CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go -C desktop test -mod=readonly ./...` | PASS |
| `CGO_ENABLED=0 GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go -C desktop vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go run -mod=readonly ./internal/archcheck` | PASS — `architecture dependencies: clean` |
| `git diff --check` on W11 paths | PASS |

All tests and benchmarks used temporary databases and synthetic facts. No real user data, credentials, production database, or external service was used.

## Remaining risk / unverified

- Remote CI, native arm64 execution, QEMU, packaging, and Arch+niri/Wayland/fcitx5/hardware-scaling GUI gates were not run.
- The existing HTTP server and TUI callers intentionally retain compatibility delegates and therefore use `context.Background()`; this W11 slice only wires cancellation into the Native Desktop task path requested by the ticket.
- Benchmark values are single-run measurements and should not be treated as stable performance baselines.
