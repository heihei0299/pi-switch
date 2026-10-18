# WP-05 Native basic progress — initial and follow-up 2026-10-08

**Status: IN PROGRESS.** The native overview is opt-in; the existing WebView remains the default and fallback. Gate B is not passed.

## Implemented

- Added an optional MyGo native overview window with Proxy status/actions, a searchable basic profile list, a read-only current-profile summary, and Proxy/Web UI address summaries.
- Proxy results, tray/overview errors, and settings loading feedback use MyGo status roles for assistive technology; physical screen-reader behavior is still unverified.
- Reads shared config types and reuses the existing daemon service. It omits API credentials and URL userinfo/path/query; selecting a profile updates only `current` through the shared cross-process config lock and rejects stale selections.
- Failed overview refreshes retain the last successful snapshot and profile selection while surfacing the error; a successful refresh replaces the snapshot and resets selection.
- Kept the existing WebView accessible from both the application and tray menus; closing the native overview hides that window without quitting the app.

## Verification

| Check | Result |
| --- | --- |
| `GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 go -C desktop test -mod=readonly ./...` | PASS |
| `GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 go vet -mod=readonly ./...` (`desktop/`) | PASS |
| `./node_modules/.bin/mygo vet .` (`desktop/`) | PASS |
| Native profile switch against temporary `PI_SWITCH_CONFIG` (including stale profile and credential-preservation cases) | PASS |
| Active profile summary against isolated config: API/model counts and endpoint host displayed; API key, URL userinfo, path and query omitted | PASS |
| Native overview refresh state: failed refresh retained visible last-good profile/address data and selection; successful refresh replaced the snapshot and cleared the error | PASS |
| `outdir=$(mktemp -d /tmp/pi-switch-wp05.XXXXXX) && GOTOOLCHAIN=local GOPROXY=off go -C desktop build -mod=readonly -o "$outdir/pi-switch-desktop" .` | PASS; output kept outside the worktree |
| UI tester: empty/error config, credential omission, Chinese IME composition simulation, 1,000-row virtualized list, keyboard Home/Up/Down/End navigation, long names, dark theme and scale 2 | PASS; the list-navigation test was rerun after adding Home/Up/Down assertions |

## Not verified / remaining

- Interactive GUI, real fcitx5 composition/candidate placement, screen-reader behavior, and physical HiDPI/input behavior were not verified: this session has no `DISPLAY`/`WAYLAND_DISPLAY`, and Xvfb/xdotool are unavailable.
- Gate B all-page parity and no-P0/P1 review remain incomplete. The default page was not switched.
- WebView/native startup, idle resource usage, and scrolling were not repeatedly benchmarked; Windows/macOS GUI checks remain unavailable on this host.
- Full repository tests are deferred until WP-05 through WP-08 are complete, as requested.
