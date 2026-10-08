# MyGo desktop shell — test distribution only

This is still the isolated app `com.heihei0299.piswitch.mygo-spike`, not a supported or public desktop release. WP-05/06 native views are previews; the React WebView remains the real management UI. CI artifacts are uploaded only as short-lived GitHub Actions artifacts and are never published to npm or a GitHub Release.

The desktop package embeds a platform-matched `pi-switch` CLI resource. The app reuses the existing daemon service; closing its windows does not stop Proxy. Linux GTK 3 and WebKitGTK are required. AppIndicator is optional because the File menu is the fallback. Windows requires the WebView2 runtime. macOS test builds are not Developer ID signed/notarized, and Windows builds have no publisher signature; neither should be installed or redistributed as a release.

No automatic-update channel is configured. Linux tarball install/reinstall/uninstall and an isolated 0.1.0→0.1.1 replacement with sentinel-data preservation were tested in temporary homes; Debian package-manager transactions, system rollback, daemon lifecycle, and real-user data migration remain unverified. The existing CLI/npm package and its six-target build workflow are unchanged.

## Build

The [Desktop CI workflow](../.github/workflows/desktop.yml) builds Linux, Windows, and macOS test artifacts from a clean checkout, bundles the matching CLI, and retains the outputs as Actions artifacts for seven days. It does not publish them.

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

Do not point test builds at real user paths. The desktop workflow is configured to validate compilation, UI tests, and the Linux tarball install/reinstall/uninstall lifecycle in an isolated `HOME`; remote Actions execution, interactive GUI, real IME/high-DPI, Debian package-manager install/rollback, daemon lifecycle, and supported-platform smoke tests remain unverified.
