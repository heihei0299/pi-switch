# Architecture V2 W03 — Gateway rules and publication — 2026-10-13

**Status:** W03 acceptance passed locally. Previous ticket: W02 `7e73ef0`. No push, live `models.json`, or manual GUI smoke was run.

## Implementation

- Pure Gateway planning/validation/diff/merge/selection rules now live in `internal/domain/gateway`; protocol identifiers/capabilities live in `internal/domain/protocol`.
- Catalog snapshot parsing, lookup, and fill policies now live in `internal/domain/catalog`. `internal/adapters/outbound/catalog` owns models.dev fetch/cache/TTL/stale fallback; `internal/catalog` remains a compatibility facade for the profile-fetch path.
- `internal/usecase/gateway` owns catalog enrichment policy, generated/draft/selected plan construction, and explicit publish. Its small Store and Catalog ports are wired by `internal/bootstrap`.
- `internal/adapters/outbound/filegateway` is the sole `models.json` reader/writer. It preserves the stable cross-process lock, stale-snapshot rejection, third-party data carried by the plan, atomic replacement, and mode `0600`; its lock wait is context-cancellable. A zero-pending plan returns without opening/creating the file or lock.
- HTTP preview/publish and Native Gateway flows now use the Gateway usecase through the application facade. `internal/gateway` stays as a compatibility facade for remaining CLI/TUI consumers.
- Added a no-op publish regression that checks file bytes and mtime, plus usecase tests proving no-op/cancel do not call storage, publish uses the previewed plan, and generated vs. draft catalog-fill policies stay distinct. Adapter tests cover lock serialization and cancellation without file changes.

## Verification

| Check | Result |
|---|---|
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (`desktop/`) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (root) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (`desktop/`) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -race -mod=readonly ./internal/adapters/outbound/filegateway ./internal/usecase/gateway ./internal/gateway ./internal/server` | PASS (server package 110.749s) |
| `GOTOOLCHAIN=local GOPROXY=off go run ./internal/archcheck` | PASS — clean |
| `git diff --check` | PASS |

The root/Desktop suites retain coverage for third-party provider and metadata preservation, stale preview rejection, `models.json` `0600` publication, catalog expiry/stale fallback, HTTP behavior, Native cancellation/no-op, and unchanged config bytes.

## Remaining limits

- CLI/TUI still call the `internal/gateway` compatibility facade; it delegates to the same usecase/adapter, and direct caller cleanup remains for W08.
- No remote CI, manual GUI smoke, or live user `models.json` was used for this ticket.
