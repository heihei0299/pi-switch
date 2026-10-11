# MyGo desktop shell — test distribution only

This is still the isolated app `com.heihei0299.piswitch.mygo-spike`, not a public desktop release. Supported desktop targets are Linux amd64 and arm64; Windows and macOS desktop builds are not supported. The React WebView remains the default management UI; `--native` is an opt-in validation entry for the seven T02 standard pages, not the T04 default switch. WP-05/06 complex native views remain previews. CI artifacts are uploaded only as short-lived GitHub Actions artifacts and are never published to npm or a GitHub Release.

The desktop package embeds a platform-matched `pi-switch` CLI resource. The app reuses the existing daemon service; closing its windows does not stop Proxy. Linux GTK 3 and WebKitGTK are required. AppIndicator is optional because the File menu is the fallback. New CLI/npm versions support Linux amd64 and arm64 only; macOS and Windows are retired for new versions.

No automatic-update channel is configured. Linux tarball install/reinstall/uninstall and an isolated 0.1.0→0.1.1 replacement with sentinel-data preservation were tested in temporary homes; Debian package-manager transactions, system rollback, daemon lifecycle, and real-user data migration remain unverified. New CLI/npm packages contain only Linux amd64 and arm64 binaries; historical release artifacts are unchanged.

## Build

The [Desktop CI workflow](../.github/workflows/desktop.yml) builds Linux amd64 and arm64 test targets from a clean checkout. It bundles the matching CLI and retains outputs as Actions artifacts for seven days; it does not publish them.

A local `mygo build` writes `webui/dist`, `desktop/dist`, `desktop/build`, and a CLI binary under `desktop/resources`; use a disposable worktree rather than overwriting existing generated files. The build requires the locked desktop Bun dependencies and the WebUI npm dependencies. MyGo's `buildCommand` builds the embedded WebUI and desktop frontend.

For development without touching real data, use a CLI from this checkout and fresh temporary paths. Keep the same isolated environment for both the desktop app and CLI invocation:

```bash
data_dir=$(mktemp -d /tmp/pi-switch-mygo.XXXXXX)
mkdir -p "$data_dir/config" "$data_dir/xdg"
cd desktop
PI_SWITCH_CLI_PATH=/path/to/pi-switch \
PI_SWITCH_CONFIG_DIR="$data_dir/config" \
PI_SWITCH_CONFIG="$data_dir/config/config.json" \
PI_SWITCH_DB="$data_dir/config/requests.db" \
PI_SWITCH_MODELS="$data_dir/models.json" \
XDG_CONFIG_HOME="$data_dir/xdg" \
bun run dev
```

Do not point test builds at real user paths. The desktop workflow is configured to validate compilation, UI tests, and the Linux tarball install/reinstall/uninstall lifecycle in an isolated `HOME`; remote Actions execution, interactive GUI, real IME/high-DPI, Debian package-manager install/rollback, daemon lifecycle, and Linux desktop GUI smoke tests remain unverified.

## Native-only validation entry (T02)

Start a **fresh desktop process** with `--native` to create a MyGo widget window without registering the legacy management router/protocol or creating a React WebView window. Exit any previous desktop instance normally first: a second invocation is forwarded to the existing process, which may already have initialized the legacy UI. The default launch and File/tray native entry remain available for migration comparison. `--start-minimized` still requires a working tray; without one, the window stays visible.

With the existing Go module/build caches and Linux runtime dependencies present, the Go entry below does not run the React/Bun frontend build. It deliberately disables dependency downloads; if dependencies are missing, stop rather than install them without approval. The binary still links the transitional server/WebUI package, whose package initialization fingerprints embedded assets; this is not removal of WebView/React build or runtime dependencies.

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
