# Architecture V2 W19 — final integration acceptance — 2026-10-14

**Status:** W19 local integration acceptance passed for the executable local gates listed below. No production business implementation was changed. No test, validation script, or architecture-document fix was required. No push was performed.

This report is the only W19 source change. The worktree contained pre-existing user changes before W19; they were not staged, overwritten, or included.

## Acceptance boundary

W19 revalidated the W15–W18 chain at `HEAD` and checked the requested compatibility, cancellation, export, streaming, usage/cost, and two-module architecture contracts:

| Work order | Commit | Scope | Result |
|---|---|---|---|
| W15 | `f973a6dffda2a0b750abe1afcebce166c238ed28` | Analytics exports use request context; context-aware aggregation/export tests | PASS |
| W16 | `39517e5ad256c7754484c13d68d07d9b7d0f5660` | Bounded SSE relay/parser buffers and long-stream coverage | PASS |
| W17 | `9b0e2c54cd4335ac1455b245e994ca4ae2eb5921` | HTTP response writes remain in the outbound proxy adapter | PASS |
| W18 | `d1581ceb80a69e33fc75acc67d68e30d8122b985` | Root and Desktop two-module architecture scan, including positive/negative fixtures | PASS |

`HEAD` was `d1581ceb80a69e33fc75acc67d68e30d8122b985` before the W19 report commit.

## Executed verification

All Go commands used the existing local module cache with `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off`; no dependency installation or network fetch was performed.

### Root and Desktop tests/build checks

| Command | Result | Measured result |
|---|---|---|
| `go test -mod=readonly -count=1 ./...` | PASS | 13.280 s |
| `go vet -mod=readonly ./...` | PASS | 0.158 s |
| `CGO_ENABLED=0 go -C desktop test -mod=readonly -count=1 ./...` | PASS | 0.821 s |
| `CGO_ENABLED=0 go -C desktop vet -mod=readonly ./...` | PASS | 0.078 s |
| `go -C desktop run -mod=readonly github.com/egoist/mygo/cmd/mygo vet .` | PASS | 0.480 s |
| `bash scripts/check-secrets.sh` | PASS | 0.042 s |
| `node --check bin/pi-switch.js` | PASS | 0.016 s |

The root suite passed all root packages, including `internal/server`, `internal/usecase/inference`, `internal/usecase/analytics`, `internal/domain/usage`, `internal/archcheck`, and the CLI command. The Desktop module passed its package test suite.

### Two-module architecture check

| Command | Result | Evidence |
|---|---|---|
| `go test -mod=readonly ./internal/archcheck -count=1 -v` | PASS | Root positive/negative fixtures and Desktop positive/negative fixtures all passed; 0.258 s |
| `go run -mod=readonly ./internal/archcheck` | PASS | `architecture dependencies: clean (2 modules)`; 0.169 s |

The architecture test covered illegal dependency directions, root/Bootstrap/command/WebUI imports of Desktop, Desktop layer violations, adapter placement, and test-only import exclusions.

### W15–W18 targeted compatibility checks

Command:

```text
go test -mod=readonly -count=1 -v \
  ./internal/domain/usage \
  ./internal/usecase/inference \
  ./internal/usecase/analytics \
  ./internal/adapters/outbound/proxy \
  ./internal/server \
  -run 'Test(Anthropic|ExtractUsage|UsageKnown|SseUsage|RelayStream|ExecuteStream|Stats|Summary|ExportRequests|ContextQuery|Service|StreamResponse|WriteResponse|StatsHandlers)'
```

Result: **PASS**, 4.613 s.

The executed tests directly covered:

- Context cancellation through Stats, conversations, request details, JSON export, and CSV export handlers.
- Export aliases, unknown values, ordering, JSON streaming with bounded writes, and writer cancellation.
- SSE pass-through byte ordering, terminal-event handling, upstream/read/write failure handling, cancellation before terminal output, and upstream close behavior.
- Flush behavior, usage extraction, cache/reasoning fields, explicit zero versus unknown usage, and cost calculation.
- Oversized SSE frame rejection for pass-through and converted streams.
- Proxy adapter ownership of response writing and flushing.

### Race verification

Command:

```text
go test -race -mod=readonly -count=1 \
  ./internal/domain/usage \
  ./internal/usecase/inference \
  ./internal/usecase/analytics \
  ./internal/adapters/outbound/proxy \
  ./internal/server
```

Result: **PASS**, 119.329 s. Individual package results were:

- `internal/domain/usage`: 1.076 s
- `internal/usecase/inference`: 25.214 s
- `internal/usecase/analytics`: 1.454 s
- `internal/adapters/outbound/proxy`: 1.007 s
- `internal/server`: 114.176 s

No race report was emitted.

### 32 MiB live-heap stream pressure

Command:

```text
go test -mod=readonly ./internal/usecase/inference \
  -run '^TestRelayStreamLongPassThroughDoesNotRetainWholeResponse$' -count=1 -v
```

Result: **PASS**, 1.787 s.

The generated stream was **33,554,430 bytes** (`32 MiB` rounded to a complete 10-byte SSE frame). The test's forced-GC live-heap sampling reported **7,024 bytes peak heap delta**, below the test limit of **8 MiB**. This is an in-process live-heap measurement, not an OS RSS measurement; the stream body is a generated local fixture, not a remote provider.

### W11 Analytics benchmark rerun

Command:

```text
go test -mod=readonly ./internal/usecase/analytics -run '^$' \
  -bench '^BenchmarkStats(History|HistorySelectiveWindow|AndConversations)$' \
  -benchtime=1x -benchmem -count=1
```

Result: **PASS**, 71.907 s wall time. Environment: Linux amd64, Intel Core Ultra 5 225H, `go1.27.2-X:nodwarf5`.

| Benchmark | 10k | 100k | 1M |
|---|---:|---:|---:|
| `StatsHistory/source_off` | 18.400 ms / 5.35 MB | 179.487 ms / 52.87 MB | 1.789 s / 528.07 MB |
| `StatsHistory/session_scan_128` | 22.251 ms / 7.07 MB | 216.114 ms / 69.71 MB | 2.160 s / 696.11 MB |
| `StatsHistorySelectiveWindow` | 1.170 ms / 0.12 MB | 4.636 ms / 0.60 MB | 31.670 ms / 5.35 MB |
| `StatsAndConversations/separate_scans` | 43.178 ms / 14.06 MB | 422.823 ms / 139.34 MB | 4.205 s / 1.392 GB |
| `StatsAndConversations/shared_window` | 21.892 ms / 7.07 MB | 217.026 ms / 69.71 MB | 2.173 s / 696.11 MB |

Values are the benchmark's single measured iteration and allocation output; they are repeatable evidence for this checkout, not a claim of production telemetry or a percentage improvement over an unmeasured baseline.

### CLI smoke and distribution builds

A current root CLI was built to `/tmp/pi-switch-w19/pi-switch`, leaving the repository's existing ignored `bin/pi-switch` untouched:

```text
bash scripts/build-go.sh /tmp/pi-switch-w19/pi-switch
bash scripts/functional-check.sh /tmp/pi-switch-w19/pi-switch
```

Result: **PASS**. The functional smoke covered build identity, API-only management routing, legacy v1 config migration, legacy SQLite/request-log compatibility, file modes, and managed headless Proxy start/status/health/stop. Fresh amd64 CLI SHA-256:

```text
3c82e91921f40092a29d60c27353b3f679fbc9962ff0c866604e52f0da297f62
```

The Node launcher smoke also passed:

```text
node --test scripts/pi-switch.test.js
```

Result: **PASS, 8/8 tests**.

Fresh clean-worktree cross-build verification was run from detached `HEAD d1581ce` so package outputs did not touch this worktree:

- Root `linux/amd64`: PASS; `--build-info` reported commit `d1581ce`, `dirty=false`, target `linux/amd64`; SHA-256 `426fe1a1bd178d3328fb0a4b24814ef13e1a1f12acc472d1182cefb5f7bac5d5`.
- Root `linux/arm64`: PASS; `file` reported an AArch64 ELF and embedded commit/target strings matched `d1581ce` / `linux/arm64`; SHA-256 `48116d94949f95f06cf1c29bcb65f5fe14bf5df40c6a9b27207b44887ff5a197`.
- Desktop `linux/amd64`: PASS; MyGo emitted Native executable, matching CLI, `.deb`, tarball, installer, desktop file, icon, and build identity; Native executable SHA-256 `1412518e747776e886bae73d1152e5644715a55b4013a03a0a19e02d8cca7c65`.
- Desktop `linux/arm64`: PASS; MyGo emitted the same package set; `file` reported AArch64 executables; Native executable SHA-256 `1ad6ade944fa90d89adb972dca001ad0b6d157990989c0b81ef8e02c045ede0a`.

The fresh amd64 archive installer was also tested with:

```text
bash desktop/scripts/test-linux-installer.sh \
  /tmp/pi-switch-w19/install.sh /tmp/pi-switch-w19/app.tar.gz
```

Result: **PASS**. Install, reinstall, uninstall, malicious symlink archive rejection, and user-data preservation passed in a temporary home. Inputs were fresh current-HEAD package outputs; installer SHA-256 `72c28fd7452f7280d61b1f7b500b5fdba869a74542b434edf353d48a8ae3d6b3`, archive SHA-256 `83508264c6513debfaa1f097ea93d78a63206f6d22b7bbc554a8d2c172408ef2`.

One preliminary combined build harness attempted to execute the cross-compiled arm64 CLI on this amd64 host and stopped with the expected shell `Exec format error`. It was a harness mistake, not a product/test failure; the corrected arm64 verification used ELF inspection and embedded identity checks, and the complete corrected cross-build run passed.

## P0/P1 findings

- **P0:** none observed.
- **P1:** none observed.
- No production implementation, production test behavior, or validation script required correction.

## Worktree and change boundary

The following pre-existing worktree state was recorded before W19 and was left untouched:

- Tracked modifications: `.gitignore`, `docs/test-reports/mygo-t02-native-standard-2026-10-10.md`, `docs/test-reports/mygo-wp-06-native-complex-2026-10-08.md`.
- Untracked paths: `.commandcode/`, `.idea/`, `AGENTS.md`, `docs/architecture-analysis-2026-09-13.md`, `docs/test-reports/mygo-wp-01-spike-2026-10-08.md`, `mygo-desktop-stability-fixes.md`, `pi-switch-linux-only-retirement-plan.md`, `tmp/`, and `webui/`.

Only `docs/test-reports/mygo-rearchitecture-w19-2026-10-14.md` was added for W19. It is intended to be the only W19 commit change.

## Unverified gates and limits

These are not local W15–W19 code failures, but remain outside this acceptance:

- Remote CI, push/review/merge, release publication, deployment, and package-manager `.deb` lifecycle requiring privileged system changes.
- Native arm64 execution; the arm64 artifacts were cross-built and inspected but not executed on an AArch64 host or under QEMU.
- Arch+niri/Wayland, fcitx5/IME, hardware high-DPI scaling, tray behavior, full GUI keyboard/focus interaction, and managed-Proxy lifecycle through a real GUI session. Ubuntu/Xvfb/Openbox evidence remains a separate X11 smoke and does not satisfy those gates.
- Real user-data migration and external provider/network behavior.
- The 32 MiB stream measurement is live Go heap after forced GC, not a full process RSS/fragmentation or prolonged soak test.

## Final disposition

**W19 local integration acceptance: PASS.** The W15–W18 compatibility, cancellation, bounded stream, export, usage/cost, race, architecture, root/Desktop test, CLI smoke, and current-head build checks passed locally. The unverified environment/release gates above remain explicitly unclaimed.
