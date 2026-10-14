# Architecture V2 W08 — final local audit — 2026-10-13

**Code HEAD audited:** `80f011a` (the documentation-only audit commit after `b16c9ca`, `refactor(config): move HTTP replacement workflow`). **W08 is not marked complete:** the safe local gates below pass, but CI-runner-only and native arm64 checks remain unverified. No push or remote GitHub CI was performed.

## Scope and compatibility audit

- W08a–W08h are recorded in their individual reports and commits. CLI, HTTP, Desktop, and TUI production paths now use the gateway/provider/settings/packages/daemon/analytics services where those slices were migrated. Provider profile persistence no longer has the old `internal/profile` write path; production CLI and HTTP profile handlers no longer call its upstream probe/fetch/enrichment functions directly.
- Remaining `internal/application` workflows have production Desktop/server consumers (draft credential masking/restoration, redacted validation, backups, overview, diagnostics, and usage). The unused settings wrappers were removed. The `internal/gateway` compatibility facade remains because its package-level contract suite and Desktop/TUI test fixtures still exercise that legacy surface; production entrypoints use the new Gateway path.
- W08 introduced no HTTP route, config/schema field, SQLite persistence format, or Gateway JSON format. The W08 diff changes no route registrations or persistence/schema files. Existing CLI/API contract, migration, no-backup, revision, error-mapping, config-byte, and no-lost-update tests remain in the root suite.

## Final local verification

| Gate | Result |
|---|---|
| `GOTOOLCHAIN=local GOPROXY=off PI_SWITCH_DB=<isolated /tmp DB> go test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` | PASS |
| `CGO_ENABLED=0 go -C desktop test -mod=readonly ./...` | PASS |
| `CGO_ENABLED=0 go -C desktop vet -mod=readonly ./...` and MyGo vet | PASS |
| `go run -mod=readonly ./internal/archcheck` | PASS — clean |
| `bash scripts/check-secrets.sh` | PASS |
| Root build identity, `--help`, and `scripts/functional-check.sh bin/pi-switch` | PASS |
| `node --test scripts/pi-switch.test.js` | PASS — 8/8 |
| Root Linux amd64/arm64 builds and npm package audit | PASS in an isolated `git archive` staging tree |
| Desktop Linux amd64/arm64 package builds and installer install/reinstall/uninstall/data-preservation smoke | PASS |
| Desktop package architecture metadata (`ar`/`tar`), binary architecture (`file`), and credential-shaped package-file scan | PASS |
| Packaged Desktop amd64 CLI/API/Proxy functional smoke | PASS |
| `gofmt` on changed Go files and committed W08 `git diff --check` | PASS |

The in-place npm package audit found four ignored binaries dated 2026-08-31 (`darwin`/`windows`) already in `bin/`, so the current directory was not a clean CI package workspace. They were left untouched. Re-running the same package audit in a clean archive containing the final Linux amd64/arm64 binaries passed.

## Supplemental final-HEAD rerun

After the documentation-only commit `80f011a`, the following were rerun against that exact HEAD: full root tests/vet, full Desktop tests/vet/MyGo vet, architecture check, Secrets Scan, root build-identity/help/functional smoke, Node CLI tests (8/8), amd64/arm64 root builds and isolated npm package audit, plus Desktop amd64/arm64 package builds, architecture metadata checks, installer smoke, and packaged amd64 CLI/API/Proxy smoke. All passed. The isolated root test DB was `/tmp/pi-switch-w08-head.JbVpfq/requests.db`; the clean npm package audit tree was `/tmp/pi-switch-npm-head.CWmneY/`.

The packaged arm64 CLI's `--version`, `--build-info`, and `--help` also passed under `qemu-aarch64-static`, reporting `linux/arm64` and commit `80f011a`. This is emulation evidence only, not a substitute for the native arm64 CI run.

## Gates not verified / limits

- Actual Root/Desktop GitHub Actions were not triggered. The local Node runtime is v26, while the workflows specify Node 22 (Root) and Node 24 (Desktop).
- Debian package install/remove lifecycle was not run: its script explicitly refuses outside disposable GitHub runners and performs system package install/purge. `dpkg-deb` is also unavailable locally; package metadata was instead verified from the `.deb` control archive.
- Native arm64 CLI execution, packaged arm64 window/Xvfb smoke, and arm64 rollback lifecycle were not run on this x86_64 host. No remote GUI container was modified.
- Arch+niri/Wayland, fcitx5, and hardware scaling remain unverified; the available Ubuntu/Xvfb environment cannot substitute for those gates.
- The isolated Go test DB (`/tmp/pi-switch-w08-final.2GGQjz/`) and npm package-audit staging tree (`/tmp/pi-switch-npm-package.DljTT5/`) were left in place; no cleanup was performed.

Because the W08 Desktop CI gate still includes unverified runner-specific checks, W08 remains in progress rather than being reported as fully passed.
