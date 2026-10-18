# WP-01 MyGo feasibility spike — 2026-10-08

**Status: PARTIAL / Gate A BLOCKED.** A separate Linux spike exists and builds. It is not a completed WP, not a Windows GUI result, and not approval to start WP-02 or distribute an app.

## Version and isolation

- MyGo: `v0.3.3`, peeled tag commit `e25a1fc282f93f8af4b7c2ece0b133f62f277265`.
- `desktop/go.mod` isolates Go `1.27.1` from root `go.mod` (`1.24.2`); it pins `github.com/egoist/mygo v0.3.3`.
- `desktop/package.json` pins `mygo-cli` and `mygo-runtime` at `0.3.3`; `bun.lock` locks platform CLI packages. Both npm packages report MIT; the Go module includes `LICENSE`.
- App identifier is `com.heihei0299.piswitch.mygo-spike`. The WebView receives only built static files through `PI_SWITCH_SPIKE_WEBUI_DIST` and a custom scheme. No pi-switch management API, config, model registry, request DB, or Pi session is opened by this spike.
- Linux runtime checks used a temporary `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, and `XDG_CACHE_HOME`; the built React files were in `/tmp/pi-switch-wp00.3NUESh/webui-dist`.

## What exists

`desktop/` contains three windows: a generated TypeScript Hello/Bind page, native Go controls with a text input and buttons, and the existing React static WebUI. `src/mygo.ts` is generated from `Greeter`; TypeScript calls `Greeter.greet` and `Greeter.info`. `main_test.go` exercises the native view using MyGo's windowless UI tester. The app requests MyGo's single-instance lock and focuses the Hello window on a second-instance event. The Bind demo proves typed values only; error and cancellation semantics remain untested.

## Checks run

| Check | Result |
| --- | --- |
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./...` (in `desktop/`) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./...` (in `desktop/`) | PASS |
| `./node_modules/.bin/mygo vet` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off bun run generate` | PASS |
| `bun run typecheck` | PASS |
| `bun run build:web` | PASS |
| `bun run build` (`GOTOOLCHAIN=local`, `GOPROXY=off`; existing Go caches) | PASS for Linux/amd64; produced local unsigned executable, `.deb`, and tarball under ignored `desktop/build/` |
| `./node_modules/.bin/mygo build -platform windows/amd64 -o /tmp/pi-switch-wp00.3NUESh/mygo-windows --skip-build-command` | PASS as cross-compilation only; Windows installer skipped because NSIS is unavailable |
| MyGo source-only high-signal secret scan | PASS, no matches |
| MyGo dependency license evidence | `mygo-cli@0.3.3` MIT, `mygo-runtime@0.3.3` MIT, Go module `LICENSE` present |

`mygo doctor` found Go `1.27.1`, Bun `1.4.2`, GTK 3 and WebKitGTK 4.1 (`2.52.6`). `ayatana-appindicator3-0.1` is present, but `libayatana-appindicator3-0.1` is not. The spike does not use a tray and its Linux dev windows launched without one. Missing-WebKit and portal-fallback scenarios were not exercised.

A Linux `mygo dev` run created the Hello, native-controls, and React WebView windows under niri. That confirms a dev-mode launch and static page load only. It does **not** verify text composition, focus/menu/paste, candidate placement, click alignment, or absence of visual artifacts. `fcitx5-remote -n` reported Rime, but no real Chinese composition was performed.

## Required blocks / not verified

- **AT-11 BLOCK:** no interactive keyboard/input-method or visual scale/click/scroll inspection. The host reports 3200×2000 physical, 1600×1000 logical, scale 2; that alone is not a pass.
- **AT-12 BLOCK:** this host is Linux only. Windows WebView2/IME/high-DPI and macOS GUI behavior were not tested. The Windows cross-build is not a substitute.
- **AT-13 PARTIAL:** Linux GTK/WebKit dependencies were identified and the trayless dev app launched, but missing-WebKit/portal/shortcut degradation cases were not run.
- **AT-14 BLOCK:** no five-run cold/warm timing, 60-second idle CPU/RSS, or 100/1000-row scroll dataset was collected.
- **Single instance / production lifecycle BLOCK:** the code uses MyGo's lock API, but a runtime relaunch check did not produce a reliable pass; a packaged-app launch also exited before a visible window was confirmed. No further system-level process/lock inspection was performed.
- **Distribution boundary:** the generated Linux package contains the spike executable and icon, while the existing React assets remain external at `PI_SWITCH_SPIKE_WEBUI_DIST`; the package is not self-contained.
- **Gate A BLOCK:** the required Linux interactive evidence is incomplete and no Windows GUI host is available. Do not enter WP-02 or batch UI migration until the blocked tests are run on real supported hosts.

The Linux `.deb` and tarball are unsigned local test artifacts only; none were installed, published, or released. Generated/ignored `desktop/build`, `desktop/dist`, `desktop/node_modules`, and `.mygo` outputs are intentionally retained. No root dependency lockfile or user configuration/data was changed by WP-01.
