#!/usr/bin/env bash
set -euo pipefail

app=${1:?usage: test-native-window-xvfb.sh packaged-app}
log=${2:?usage: test-native-window-xvfb.sh packaged-app app-log}
test -x "$app"
work_dir=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/pi-switch-native-xvfb.XXXXXX")
mkdir -p "$work_dir/home" "$work_dir/xdg/config" "$work_dir/xdg/data" "$work_dir/runtime" "$work_dir/config" "$work_dir/agent/sessions"
chmod 0700 "$work_dir/runtime"
export HOME="$work_dir/home"
export XDG_CONFIG_HOME="$work_dir/xdg/config" XDG_DATA_HOME="$work_dir/xdg/data" XDG_RUNTIME_DIR="$work_dir/runtime"
export PI_SWITCH_CONFIG_DIR="$work_dir/config" PI_SWITCH_CONFIG="$work_dir/config/config.json"
export PI_SWITCH_DB="$work_dir/config/requests.db" PI_SWITCH_MODELS="$work_dir/models.json"
export PI_AGENT_ROOT="$work_dir/agent" PI_AGENT_SETTINGS="$work_dir/agent/settings.json"
export GDK_BACKEND=x11
unset WAYLAND_DISPLAY

xvfb-run -a -s '-screen 0 1600x1000x24' bash -euo pipefail -c '
  app=$1
  log=$2
  "$app" >"$log" 2>&1 &
  pid=$!
  cleanup() {
    kill -TERM "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  }
  trap cleanup EXIT
  for _ in $(seq 1 100); do
    if ! kill -0 "$pid" 2>/dev/null; then
      cat "$log" >&2
      exit 1
    fi
    if xwininfo -root -tree 2>/dev/null | grep -F "pi-switch — Native Management" >/dev/null; then
      echo "packaged Native window became visible in Xvfb"
      exit 0
    fi
    sleep 0.1
  done
  cat "$log" >&2
  echo "packaged Native window did not become visible in Xvfb" >&2
  exit 1
' _ "$app" "$log"
