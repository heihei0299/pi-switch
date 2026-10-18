# Architecture V2 W06c — shared analytics usecase and SQLite adapter — 2026-10-13

**Status:** W06c passed locally. W06 remains in progress; no push or remote CI run.

## Implementation

- Moved the conversation matcher into `internal/domain/conversation`, where its matching policy and reusable index are domain rules.
- Moved statistics aggregation, nullable fact handling, pagination DTOs, summaries, and exports into `internal/usecase/analytics`. The usecase now depends on a `Repository` port rather than `database/sql` or `internal/store`.
- Added `internal/adapters/outbound/sqlanalytics` for SQLite row reads, ordering, paging, nullable conversion, and the existing millisecond right-open timestamp filter.
- Added bootstrap wiring for management and summary analytics. HTTP, Native, and TUI entry points now use the same analytics service; session-file scanning and DB construction stay in bootstrap.
- Replaced SQLite-coupled analytics tests with usecase tests over a small in-memory repository and adapter integration tests for timestamp boundaries, nulls, ordering, and pages. No persisted schema or response DTO changed.

## Performance check

The W06a/W06b 10k/100k/1M synthetic benchmark was rerun against the new SQLite adapter. Results remain within measurement noise of W06b: 1M rows took 1.79 s with attribution off and 2.16 s with 128 session candidates (one in-window), allocating 528 MB and 696 MB respectively. Peak benchmark-process RSS was 168,904 KiB. This confirms the query-port/adapter move did not erase the matcher improvement; it is not a production-data measurement.

## Verification

| Check | Result |
|---|---|
| Analytics usecase and SQLite adapter tests | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go -C desktop vet -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go run ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |

## Remaining W06 work

- SQLite window filtering still evaluates the millisecond timestamp expression while scanning history; indexes and selective-window benchmarks remain to be evaluated.
- Native requests and conversation views can still perform separate full-history scans when their windows differ. Their combined-query/performance trade-off remains unmeasured.
- No production database or real session files were read.
