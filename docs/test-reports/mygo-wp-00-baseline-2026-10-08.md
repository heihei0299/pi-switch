# WP-00 baseline evidence — 2026-10-08

**Status:** Local baseline passed after the inherited CLI-doc test fix; GitHub CI was not triggered from `mygo`. This is not M2/release approval.

## Frozen baseline

| Item | Evidence |
| --- | --- |
| pi-switch `main` | `18f877e60ef62e294bea8ad65bfebc5838ee96c1` |
| Remote `origin/mygo` plan commit | `cad51d6150f4228d5af5e95096129cd493d6707c` |
| Current local `mygo` | `3e59c09ba0d00b26f74b59f75193172fa3d624b5` (separate inherited-test fix) |
| MyGo snapshot | `v0.3.3`, peeled tag commit `e25a1fc282f93f8af4b7c2ece0b133f62f277265`; tag confirmed with `git ls-remote --tags https://github.com/egoist/mygo.git 'refs/tags/v0.3.*'` |
| Root Go module | `go 1.24.2`; local tool `go1.27.1-X:nodwarf5 linux/amd64` |
| Local Node/npm | Node `v26.5.1`, npm `11.17.0`; workflow uses Node 22 (environment mismatch to retain in CI evidence) |
| CI triggers | `.github/workflows/ci.yml`: push to `main`/`v*`, PR targeting `main`, or `workflow_dispatch`; ordinary `mygo` pushes do not trigger it |

Historical run `37118224938` was on `main` SHA `18f877e60ef62e294bea8ad65bfebc5838ee96c1` on 2026-10-03: `verify / go and webui` failed at `TestProviderCLI_ReadmeCommandsAreRecognized`; all six binary build jobs succeeded. The failure reproduced locally: the test required provider CLI examples in both READMEs, but those examples now live in `docs/manual-test-basic.md`. Fixed by checking the manual guide as well and requiring at least one documented example across the files. Fix commit: `3e59c09`; no product behavior changed. GitHub CI remains unrun on this local branch; no push, PR, or workflow dispatch was made.

## Existing behavior surface

WebUI navigation pages: Home, Profiles, Gateway, Proxy, Packages, Stats, Backups, Settings, Doctor.

HTTP surface observed in `internal/server/server.go`:

- Health: `GET /healthz`, `GET /health`.
- Inference: `POST /v1/chat/completions`, `/v1/completions`, `/v1/responses`, `/v1/messages`; `GET /v1/models`.
- Management config/state: `/api/config` (GET/PUT), `/api/state` (GET); validation aliases `/api/config/validate` (GET), `/api/validate` (GET/POST).
- Profiles: collection GET/POST; `/:name` GET/PUT/DELETE; `/duplicate`, `/test`, `/fetch-models`, `/models`, `/expose`, `/spoof`, `/credits` with the methods registered in `server.go`.
- Presets: GET collection and `/:id`; doctor; backups; stats, conversations and conversation requests.
- Proxy: status/start/stop and failover; Gateway: get/preview/apply/publish/health/start.
- Packages: list/add/import/get/toggle/delete; cc-switch provider discovery/import; init; settings GET/PUT; config export/import/restore stubs; build info and dumps; logs export (including `/api/export` alias).
- Static WebUI: `/`, `/assets/*`, and client-side fallback.

Synthetic baseline coverage already exists as inline/temp test fixtures (no static `testdata/` corpus was present):

- Config: `internal/config/config_test.go`, `save_at_path_test.go`, `config_channel_api_test.go`.
- Gateway: `internal/gateway/gateway_test.go`, `validation_test.go`, `migration_test.go`, plus `internal/server/gateway_*_test.go`.
- Proxy/API: `internal/server/api_contract_test.go`, `protocol_response_test.go`, `responses_stream_test.go`, `translator_limit_stream_test.go`, `proxy_auth_test.go`.
- Stats: `internal/stats/service_test.go`, `unknown_usage_test.go`, `internal/server/webui_sync_stats_test.go`.

These tests use synthetic keys/data and temporary paths; no real user config, model registry, request DB, Pi session, or API key was copied into the baseline.

## Verification

All Go tests that can access config/state ran with isolated `PI_SWITCH_CONFIG`, `PI_SWITCH_DB`, `PI_SWITCH_MODELS`, and `PI_SWITCH_CONFIG_DIR` under `/tmp/pi-switch-wp00.3NUESh`; the project lockfiles were read-only for tests/builds.

| Check | Result |
| --- | --- |
| `go test -mod=readonly ./...` | PASS, all packages; run with `GOTOOLCHAIN=local GOPROXY=off` and the four temp-path variables |
| `go vet -mod=readonly ./...` | PASS |
| `npm --prefix webui run typecheck` | PASS |
| `npm run test:webui` | PASS, 36 files / 286 tests |
| `npm --prefix webui run test:e2e -- --output=/tmp/pi-switch-wp00.3NUESh/e2e-results` | PASS, 5 passed / 3 intentionally skipped; first attempt was blocked only because the matching Playwright Chromium was absent, then the pinned browser was downloaded to the user cache and the suite rerun |
| `npm --prefix webui run build -- --outDir=/tmp/pi-switch-wp00.3NUESh/webui-dist` | PASS; kept output outside the existing ignored `webui/dist` |
| `bash scripts/check-secrets.sh` | PASS, no matches (output redacted) |
| Go build identity + CLI smoke | PASS from temp binary: `--help`, `--version`, `--build-info`, `config path`, `gateway status`; isolated config paths used |
| Six-target Go build | PASS: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64 |
| Clean npm package dry-run | PASS from a clean local clone after building current assets: 13 files / 37,925,696 bytes; six required binaries present; forbidden paths absent. Linux build identity was commit `3e59c09`, `dirty=false`, target `linux/amd64` |
| Dependency/lock evidence | Go module graph: 90/90 modules had license evidence (one README-only declaration); npm lock metadata: 121/121 root and 282/282 WebUI packages had license fields. No versions were upgraded. |

Lockfile SHA-256 at baseline: `go.sum` `bed84a935054aa72893fcaa4331e72fffca29fd6b304a0194f2d1a9c46a261bf`; root `package-lock.json` `8b19ce2ccad6cf846e7dbdaaf0fec88ac3216ee7efc459127b6b373cc142ff0c`; `webui/package-lock.json` `529b06f67f3888f5166eae44fad8e262f17daa886fa018fecc2c627a98e80325`.

**Packaging caution:** a dry-run directly in the pre-existing developer worktree included ignored binaries from September; its Linux binary identified as version `20260912.1.0`, commit `956cff3`. Those files were not overwritten or included in the clean-clone check. Release packaging must continue to use fresh CI artifacts, as the workflow currently does.

## Gate notes

- Headless Go/API tests, WebUI tests, smoke, and cross-build are covered separately from interactive GUI testing.
- This host is Arch Linux with niri/Wayland, 3200×2000 physical / 1600×1000 logical at scale 2, fcitx5/Rime, GTK 3 and WebKitGTK 4.1 available. AppIndicator is unavailable. These facts are for WP-01 only; no Windows or macOS GUI host is available.
- No GitHub CI run was triggered from `mygo`; it requires a later authorized workflow dispatch or PR/push. The historical main-branch failure is fixed in the local branch but must be confirmed by CI before M2/merge.
- Unsigned distribution was not produced; this report grants no release/signing approval.
