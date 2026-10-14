# Architecture V2 W14 — final acceptance — 2026-10-14

**Status:** W14 local acceptance passed. This report is the only W14 source change. No production code, user changes, credentials, real-user data, remote branch, or remote CI state was modified. No push was performed.

## Acceptance boundary

W14 reviewed W09–W13 scope and evidence, rechecked the Architecture V2 compatibility contracts, and ran the repository's executable local validation. The worktree already contained user changes before W14; they were not staged or included.

### W09–W13 commit and scope audit

| Work order | Commit | Scope audit | Result |
|---|---|---|---|
| W09 | `dc8a186356d4d07c3672919ddb63f2504c1a7da4` | `internal/server/config_replacement_contract_test.go` and the W09 report only; no production code | PASS |
| W10 | `34b5d2fd4b1dfa2c194f926bfa487077669ddd78` | Inference transport seam, Proxy outbound adapter, direct server/domain wiring, and regression tests/report | PASS |
| W11 | `9d73194fe19fbe53ee7d4c828f98486064b9a0e0` | Analytics use case, SQLite analytics adapter, Native Stats caller, benchmarks/tests/report | PASS |
| W12 | `f7257d232c228f3c64acefa3cfbb71c62f9a0393` | Compatibility facade and its direct test callers/tests/report; no unrelated production migration | PASS |
| W13 | `5051f8c78483d7a3aca8d6b6ac133d2857510e66` | Architecture-check tests, Desktop CI invocation, and report only | PASS |

The commits are a linear chain on `mygo`, ending at `5051f8c78483d7a3aca8d6b6ac133d2857510e66`. The commit contents were inspected with `git show`; no W14 change was mixed into an earlier commit.

## Compatibility-contract audit

The following contracts were checked against the reports, implementation, tests, and architecture guard:

- **Inference:** use case transport is HTTP-neutral; HTTP request construction and response adaptation stay in `internal/adapters/outbound/proxy`. Existing SSE conversion, retry/fallback, cancellation, headers/UA/affinity, usage extraction, and cost behavior remain covered by regression tests. `internal/usecase/inference` has no `net/http` import.
- **Analytics:** context-aware query methods propagate cancellation through the use case and SQLite adapter; legacy methods remain compatibility delegates. Cancellation tests and legacy/context result-equivalence tests pass.
- **Gateway compatibility:** `internal/gateway` remains an intentionally thin legacy facade. The production tree has no direct `internal/gateway` callers; the remaining references are compatibility tests and architecture-check fixtures. Canonical Gateway rules remain in the Domain/Bootstrap path.
- **Architecture:** `internal/archcheck` accepts the current production tree and rejects the tested illegal directions: Domain/Use Case infrastructure imports, Use Case-to-adapter imports, inbound/outbound crossing, legacy bypasses, misplaced adapters, root/Bootstrap/command/WebUI Desktop imports, and production-vs-test import confusion.
- **Operational behavior:** existing HTTP/CLI/TUI/Proxy/Gateway/SQLite/Native pathways remain covered by the root and Desktop test suites, functional CLI/API/Proxy smoke, and W09–W13 regression tests.

## Executed verification

All commands used the local locked modules with `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off` where applicable. Stateful tests used temporary paths or the repository's isolated fixtures.

| Check | Result |
|---|---|
| `go test -mod=readonly ./internal/archcheck -count=1 -v` | PASS — all architecture positive/negative fixtures, including W13 cases |
| `go run -mod=readonly ./internal/archcheck` | PASS — `architecture dependencies: clean` |
| `go vet -mod=readonly ./...` | PASS |
| `go test -mod=readonly ./...` | PASS |
| `CGO_ENABLED=0 go -C desktop test -mod=readonly ./...` | PASS |
| `CGO_ENABLED=0 go -C desktop vet -mod=readonly ./...` | PASS |
| `go -C desktop run -mod=readonly github.com/egoist/mygo/cmd/mygo vet .` | PASS |
| `go test -mod=readonly ./internal/adapters/outbound/daemon` | PASS |
| `bash scripts/check-secrets.sh` | PASS |
| `npm run build:go` | PASS |
| `bash scripts/functional-check.sh bin/pi-switch` | PASS — API-only management router, legacy config/SQLite/log migration, managed Proxy lifecycle |
| `node --test scripts/pi-switch.test.js` | PASS — 8/8 |
| `scripts/test-limited.sh` | PASS — Go suite and CLI smoke; environment reported no systemd user scope, so it used its documented nice/timeout fallback |
| W11 analytics benchmark (`-benchtime=1x`, 10k/100k/1M) | PASS — 71.518s; all benchmark groups completed |
| Root Linux cross-build (`linux/amd64`, `linux/arm64`) | PASS — commit and target identities embedded |
| Desktop package build (`linux/amd64`, `linux/arm64`) | PASS — MyGo emitted executable, `.deb`, tarball, and installer for both targets |
| `desktop/scripts/test-linux-installer.sh` on the amd64 archive | PASS — install/reinstall/uninstall and user-data preservation; malicious symlink archive rejected |
| `node scripts/check-npm-package.mjs` | PASS — no missing/forbidden package files |
| `npm pack --dry-run` | PASS — package contents enumerated without publishing |
| `git diff --check` | PASS |

The root build identity reported commit `5051f8c`, target `linux/amd64`, and `dirty=true`; the dirty flag correctly reflects pre-existing user worktree changes and is not a W14 failure.

## P0/P1 assessment

- **Known P0:** none.
- **Known P1:** none.
- **Acceptance blockers introduced by W14:** none.
- **越界生产代码修复:** none required; W14 changed only this report.

## Not verified and remaining risk

These items were **not executed and are not claimed as passed**:

1. **Remote GitHub Actions/CI**, including actual GitHub runner images, permissions, caches, and the native `ubuntu-24.04-arm` runner.
2. **Native arm64 execution and QEMU execution.** Root and Desktop arm64 artifacts were cross-built and their identities/package formats were checked, but they were not executed on arm64 or under QEMU.
3. **Debian package install/reinstall/remove/purge lifecycle.** The repository lifecycle script intentionally requires `GITHUB_ACTIONS=true` and performs host package-manager operations. It was not run on this Arch development host; no sudo/package database mutation was performed. Package creation itself passed.
4. **Native GUI runtime gates:** Arch+niri/Wayland, fcitx5/IME, hardware scaling, and real display interaction. The local host did not provide `xvfb-run`/Xvfb for the packaged-window script, and no remote GUI environment was claimed as W14 evidence.
5. **Release publication, npm publish, deployment, or push.** `npm pack --dry-run` only inspected package contents; no publication or remote write occurred.

These are environment/release gates, not observed W09–W14 code failures. The local executable validation required for this worktree passed, but the unverified gates remain before claiming cross-runner, native-arm64, package-manager, or real-GUI release readiness.

## Final disposition

**W14 local acceptance: PASS.** W09–W13 reports and commits are present, scope-compatible, and locally revalidated. The complete W14 report is committed separately; no push was performed.
