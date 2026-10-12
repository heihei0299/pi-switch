#!/usr/bin/env bash
# Resource-limited test runner.
#
# Each suite runs inside its own cgroup v2 scope (via `systemd-run --user`)
# with hard memory/CPU/task caps, low priority, idle IO class and a wall-clock
# timeout. No sudo, no symlinks created, nothing installed.
#
# Usage:
#   scripts/test-limited.sh                # Go tests + CLI smoke
#   scripts/test-limited.sh go             # Go tests only
#   scripts/test-limited.sh smoke          # binary smoke only
#
# Env knobs:
#   LIMIT_MEM=1G       memory cap per suite (cgroup MemoryMax, no swap)
#   LIMIT_CPU=100%     CPU quota per suite (100% == one core)
#   LIMIT_TASKS=256    max tasks/threads per suite
#   LIMIT_WALL=300     wall-clock seconds per suite
#   GO_MAXPROCS=2      GOMAXPROCS for go test (also runs -p 1 -parallel 2)
#   SMOKE_MEM=512M     memory cap for the binary smoke suite
#   SMOKE_CPU=50%      CPU quota for the binary smoke suite
#   NO_CGROUP=1        skip cgroups; fall back to nice + timeout only
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"

LIMIT_MEM=${LIMIT_MEM:-1G}
LIMIT_CPU=${LIMIT_CPU:-100%}
LIMIT_TASKS=${LIMIT_TASKS:-256}
LIMIT_WALL=${LIMIT_WALL:-300}
GO_MAXPROCS=${GO_MAXPROCS:-2}
SMOKE_MEM=${SMOKE_MEM:-512M}
SMOKE_CPU=${SMOKE_CPU:-50%}
GOMODCACHE=$(go env GOMODCACHE)
GOCACHE=$(go env GOCACHE)

test_root=$(mktemp -d "${TMPDIR:-/tmp}/pi-switch-test-limited.XXXXXX")
mkdir -p "$test_root"/{home,config,agent/sessions,xdg/config,xdg/data,xdg/runtime}
chmod 0700 "$test_root/xdg/runtime"
trap 'rm -rf "$test_root"' EXIT
export HOME="$test_root/home" XDG_CONFIG_HOME="$test_root/xdg/config" XDG_DATA_HOME="$test_root/xdg/data" XDG_RUNTIME_DIR="$test_root/xdg/runtime"
export PI_SWITCH_CONFIG_DIR="$test_root/config" PI_SWITCH_CONFIG="$test_root/config/config.json"
export PI_SWITCH_DB="$test_root/config/requests.db" PI_SWITCH_MODELS="$test_root/models.json"
export PI_AGENT_ROOT="$test_root/agent" GOMODCACHE GOCACHE
printf '%s\n' '{"version":2,"profiles":{},"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"conversationSource":"proxy"}}' > "$PI_SWITCH_CONFIG"
chmod 0600 "$PI_SWITCH_CONFIG"

# --- suite selection -------------------------------------------------------
suites=("$@")
if [ ${#suites[@]} -eq 0 ]; then suites=(all); fi
want=()
for s in "${suites[@]}"; do
  case "$s" in
    all) want+=(go smoke) ;;
    go|smoke) want+=("$s") ;;
    -h|--help) sed -n '2,/^set -euo/p' "$0" | head -n -1; exit 0 ;;
    *) echo "unknown suite: $s (expected go|smoke|all)" >&2; exit 2 ;;
  esac
done

# --- cgroup capability probe ----------------------------------------------
have_cgroup=0
if [ "${NO_CGROUP:-0}" != "1" ] && command -v systemd-run >/dev/null 2>&1; then
  if systemd-run --user --scope -q \
    -p MemoryMax=64M -p MemorySwapMax=0 -p CPUQuota=50% -p TasksMax=64 \
    -- true >/dev/null 2>&1; then
    have_cgroup=1
  fi
fi

# run_capped <mem> <cpu> <wall> -- <cmd...>
run_capped() {
  local mem=$1 cpu=$2 wall=$3
  shift 3
  if [ "${1:-}" = "--" ]; then shift; fi
  if [ "$have_cgroup" = "1" ]; then
    systemd-run --user --scope -q \
      -p MemoryMax="$mem" -p MemorySwapMax=0 -p CPUQuota="$cpu" -p TasksMax="$LIMIT_TASKS" \
      -- nice -n 19 ionice -c3 timeout -k 5 "$wall" "$@"
  else
    nice -n 19 ionice -c3 timeout -k 5 "$wall" "$@"
  fi
}

require_exec() {
  [ -x "$1" ] || { echo "missing executable: $1" >&2; exit 1; }
}

# --- suites ----------------------------------------------------------------
suite_go() {
  if [ "$have_cgroup" = "1" ]; then
    echo "== go test ./...  [mem<=$LIMIT_MEM cpu<=$LIMIT_CPU tasks<=$LIMIT_TASKS wall<=${LIMIT_WALL}s GOMAXPROCS=$GO_MAXPROCS -p1 -parallel2] =="
  else
    echo "== go test ./...  [nice+timeout only, wall<=${LIMIT_WALL}s GOMAXPROCS=$GO_MAXPROCS] =="
  fi
  run_capped "$LIMIT_MEM" "$LIMIT_CPU" "$LIMIT_WALL" -- \
    env -i PATH="$PATH" HOME="$HOME" XDG_CONFIG_HOME="$XDG_CONFIG_HOME" XDG_DATA_HOME="$XDG_DATA_HOME" \
      XDG_RUNTIME_DIR="$XDG_RUNTIME_DIR" PI_SWITCH_CONFIG_DIR="$PI_SWITCH_CONFIG_DIR" \
      PI_SWITCH_CONFIG="$PI_SWITCH_CONFIG" PI_SWITCH_DB="$PI_SWITCH_DB" PI_SWITCH_MODELS="$PI_SWITCH_MODELS" \
      PI_AGENT_ROOT="$PI_AGENT_ROOT" GOMODCACHE="$GOMODCACHE" GOCACHE="$GOCACHE" \
      GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS="$GO_MAXPROCS" \
      go test -count=1 -p 1 -parallel 2 ./...
}

suite_smoke() {
  local bin="$ROOT/bin/pi-switch"
  require_exec "$bin"
  echo "== binary smoke  [mem<=$SMOKE_MEM cpu<=$SMOKE_CPU wall<=45s each] =="
  local c
  for c in "--help" "version" "build-info" "gateway status"; do
    echo "-- pi-switch $c"
    # shellcheck disable=SC2086
    run_capped "$SMOKE_MEM" "$SMOKE_CPU" 45 -- \
      env -i PATH="$PATH" HOME="$HOME" XDG_CONFIG_HOME="$XDG_CONFIG_HOME" XDG_DATA_HOME="$XDG_DATA_HOME" \
      XDG_RUNTIME_DIR="$XDG_RUNTIME_DIR" PI_SWITCH_CONFIG_DIR="$PI_SWITCH_CONFIG_DIR" \
      PI_SWITCH_CONFIG="$PI_SWITCH_CONFIG" PI_SWITCH_DB="$PI_SWITCH_DB" PI_SWITCH_MODELS="$PI_SWITCH_MODELS" \
      PI_AGENT_ROOT="$PI_AGENT_ROOT" "$bin" $c
  done
}

echo "repo: $ROOT"
if [ "$have_cgroup" = "1" ]; then
  echo "mode: cgroup v2 user scope (MemoryMax/MemorySwapMax/CPUQuota/TasksMax) + nice19 + idle IO + timeout"
else
  echo "mode: WARNING - systemd user scope unavailable, only nice + timeout (no memory/CPU/tasks cap)"
fi

for s in "${want[@]}"; do
  echo
  "suite_$s"
done

echo
echo "all selected suites passed."
