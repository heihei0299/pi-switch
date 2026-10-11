# MyGo desktop shell — test distribution only

This is still the isolated app `com.heihei0299.piswitch.mygo-spike`, not a public desktop release. Current CI build targets are Linux amd64 and arm64; Windows and macOS desktop builds are not supported. This target list is not a Linux-distribution support promise. The MyGo Native window is now the default and combines the standard management pages with Gateway, JSON Editor, Stats/Sessions, and Packages. `--web` explicitly selects the legacy React WebView rollback; `--native` remains accepted. The T02 and Task 1/2 functional and real-GUI gates are not all complete, so this entry switch is not release acceptance. CI artifacts are uploaded only as short-lived GitHub Actions artifacts and are never published to npm or a GitHub Release.

The desktop package embeds a platform-matched `pi-switch` CLI resource. The app reuses the existing daemon service; closing its windows does not stop Proxy. Linux GTK 3 and WebKitGTK are required. AppIndicator is optional because the File menu is the fallback. New CLI/npm versions support Linux amd64 and arm64 only; macOS and Windows are retired for new versions.

No automatic-update channel is configured. Linux tarball install/reinstall/uninstall and an isolated 0.1.0→0.1.1 replacement with sentinel-data preservation were tested in temporary homes; CI installer smoke covers amd64 only, while arm64 is build-tested without installer smoke. Debian package-manager transactions, system rollback, daemon lifecycle, and real-user data migration remain unverified. New CLI/npm packages contain only Linux amd64 and arm64 binaries; historical release artifacts are unchanged.

## Build

The [Desktop CI workflow](../.github/workflows/desktop.yml) builds Linux amd64 and arm64 test targets from a clean checkout. It bundles the matching CLI and retains outputs as Actions artifacts for seven days; it does not publish them.

Build the shared WebUI first, then run MyGo from `desktop`; the Go module pins the MyGo CLI version, so no desktop npm/Bun toolchain is needed:

```bash
npm ci --prefix webui
npm --prefix webui run build
go -C desktop run -mod=readonly github.com/egoist/mygo/cmd/mygo build -skip-notarize
```

The WebUI build remains necessary because the Go server package embeds it for the retained `--web` rollback. The Native window itself has no MyGo web frontend; MyGo writes only `desktop/build` (plus its normal temporary build files), not `desktop/dist`. Build in a disposable worktree if generated outputs must be isolated.

For development without touching real data, use a CLI from this checkout and fresh temporary paths. Keep the same isolated environment for both the desktop app and CLI invocation:

```bash
data_dir=$(mktemp -d /tmp/pi-switch-mygo.XXXXXX)
mkdir -p "$data_dir/config" "$data_dir/xdg"
PI_SWITCH_CLI_PATH=/path/to/pi-switch \
PI_SWITCH_CONFIG_DIR="$data_dir/config" \
PI_SWITCH_CONFIG="$data_dir/config/config.json" \
PI_SWITCH_DB="$data_dir/config/requests.db" \
PI_SWITCH_MODELS="$data_dir/models.json" \
XDG_CONFIG_HOME="$data_dir/xdg" \
go -C desktop run -mod=readonly github.com/egoist/mygo/cmd/mygo dev
```

Do not point test builds at real user paths. Desktop CI run `38149366162` passed on commit `7dfefd8`: linux-amd64 and linux-arm64 packages built, and the amd64 tarball install/reinstall/uninstall smoke passed in an isolated `HOME`; the arm64 installer smoke was skipped. A separate Ubuntu 24.04 Xvfb/Openbox smoke confirmed window visibility and basic Tab input only. Formal Arch+niri/Wayland, real IME/high-DPI, tray, managed-Proxy GUI lifecycle, Debian package-manager install/rollback, and real-user data migration remain unverified.

## Native default and legacy rollback (T03)

The default launch creates a MyGo widget window without registering the legacy management router/protocol or creating a React WebView window. Start with `--web` only to use the old WebView rollback. Exit any previous desktop instance normally first: a second invocation is forwarded to the existing process. File/tray show, second-instance activation, and hide/quit operate on that one primary window. `--start-minimized` still requires a working tray; without one, the window stays visible.

With the existing Go module/build caches and Linux runtime dependencies present, the Go entry below does not build a MyGo web frontend. The desktop Go module still links the transitional server package and its `webui/dist` embed for the explicit `--web` rollback, so build the shared WebUI before running it. It deliberately disables dependency downloads; if Go dependencies are missing, stop rather than install them without approval.

Run from the repository root in a real Linux GUI session. Replace the CLI path with an existing, platform-matched CLI from this checkout, and leave the isolated directory intact for evidence/recovery:

```bash
(
  export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off
  export GOMODCACHE="$(go env GOMODCACHE)" GOCACHE="$(go env GOCACHE)"
  data_dir=$(mktemp -d /tmp/pi-switch-native.XXXXXX) || exit
  mkdir -p "$data_dir/home" "$data_dir/xdg" "$data_dir/config" "$data_dir/agent/sessions" || exit
  printf 'Isolated data: %s\n' "$data_dir"
  export HOME="$data_dir/home" XDG_CONFIG_HOME="$data_dir/xdg"
  unset PI_AGENT_SETTINGS PI_CODING_AGENT_DIR
  export PI_AGENT_ROOT="$data_dir/agent"
  export PI_SWITCH_CONFIG_DIR="$data_dir/config" PI_SWITCH_CONFIG="$data_dir/config/config.json"
  export PI_SWITCH_DB="$data_dir/config/requests.db" PI_SWITCH_MODELS="$data_dir/models.json"
  export PI_SWITCH_CLI_PATH=/absolute/path/to/existing/pi-switch
  go -C desktop run . --native
)
```

Use only fake credentials and controlled local upstream fixtures. Keep these same data-path variables when checking managed Proxy with the CLI; window close/hide and GUI quit do **not** stop Proxy. Stop only the identity-verified test service explicitly. A compile or headless widget test does not establish the mandatory Wayland/niri, IME, keyboard, scaling, theme, tray, or managed-Proxy GUI gates; record missing evidence as BLOCK in the [T02 report](../docs/test-reports/mygo-t02-native-standard-2026-10-10.md).
