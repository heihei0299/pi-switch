# MyGo desktop shell (WP-02 in progress)

This remains an isolated development app (`com.heihei0299.piswitch.mygo-spike`), not a distributable desktop release. The main window can start/stop/check the existing Proxy daemon; closing the window hides it, while **Quit** exits the desktop app without stopping Proxy. Tray setup failures are shown in the window, and the File menu remains the fallback. `--start-minimized` is honored only when a tray is available.

The desktop shell reuses the root `internal/daemon` package. To start Proxy it needs a pi-switch CLI executable, found in the app resources, on `PATH`, or explicitly through `PI_SWITCH_CLI_PATH`. Until WP-07 bundles the CLI, development builds may need that environment variable.

## Run without touching user data

Use a built pi-switch CLI from this checkout and fresh temporary paths. Keep the same isolated environment for both the desktop app and any CLI invocation:

```bash
data_dir=$(mktemp -d /tmp/pi-switch-wp02.XXXXXX)
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

The app identity and XDG path remain isolated from production while Gate A is blocked. Do not point the environment variables at real user paths.

## Checks

```bash
CGO_ENABLED=0 go test -mod=readonly ./...
go vet -mod=readonly ./...
./node_modules/.bin/mygo vet .
```

Unit tests do not replace Linux/Windows GUI, tray, IME, high-DPI, crash/restart, or performance acceptance. Gate A remains blocked; this implementation is not release-ready.
