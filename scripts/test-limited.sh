#!/usr/bin/env bash
# Resource-limited test runner.
#
# Each suite runs inside its own cgroup v2 scope (via `systemd-run --user`)
# with hard memory/CPU/task caps, low priority, idle IO class and a wall-clock
# timeout. No sudo, no symlinks created, nothing installed.
#
# Usage:
#   scripts/test-limited.sh                # go + webui + tsc + smoke
#   scripts/test-limited.sh go webui       # selected suites
#   scripts/test-limited.sh smoke          # binary smoke only
#
# Env knobs:
#   LIMIT_MEM=1G       memory cap per suite (cgroup MemoryMax, no swap)
#   LIMIT_CPU=100%     CPU quota per suite (100% == one core)
#   LIMIT_TASKS=256    max tasks/threads per suite
#   LIMIT_WALL=300     wall-clock seconds per suite
#   GO_MAXPROCS=2      GOMAXPROCS for go test (also runs -p 1 -parallel 2)
#   NODE_HEAP=1024     node --max-old-space-size (MB) for vitest/tsc
#   WEBUI_MEM=1536M    memory cap for the vitest suite (node is heavier)
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
NODE_HEAP=${NODE_HEAP:-1024}
WEBUI_MEM=${WEBUI_MEM:-1536M}
SMOKE_MEM=${SMOKE_MEM:-512M}
SMOKE_CPU=${SMOKE_CPU:-50%}

# --- suite selection -------------------------------------------------------
suites=("$@")
if [ ${#suites[@]} -eq 0 ]; then suites=(all); fi
want=()
for s in "${suites[@]}"; do
  case "$s" in
    all) want+=(go webui tsc smoke) ;;
    go|webui|tsc|smoke) want+=("$s") ;;
    -h|--help) sed -n '2,/^set -euo/p' "$0" | head -n -1; exit 0 ;;
    *) echo "unknown suite: $s (expected go|webui|tsc|smoke|all)" >&2; exit 2 ;;
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
    env GOMAXPROCS="$GO_MAXPROCS" go test -count=1 -p 1 -parallel 2 ./...
}

suite_webui() {
  local bin="$ROOT/webui/node_modules/.bin/vitest"
  require_exec "$bin"
  if [ "$have_cgroup" = "1" ]; then
    echo "== vitest run  [mem<=$WEBUI_MEM cpu<=$LIMIT_CPU tasks<=$LIMIT_TASKS wall<=${LIMIT_WALL}s maxWorkers=1 nodeHeap=${NODE_HEAP}M] =="
  else
    echo "== vitest run  [nice+timeout only, maxWorkers=1 nodeHeap=${NODE_HEAP}M] =="
  fi
  (cd "$ROOT/webui" && run_capped "$WEBUI_MEM" "$LIMIT_CPU" "$LIMIT_WALL" -- \
    env NODE_OPTIONS="--max-old-space-size=$NODE_HEAP" "$bin" run --maxWorkers=1)
}

suite_tsc() {
  local bin="$ROOT/webui/node_modules/.bin/tsc"
  require_exec "$bin"
  echo "== tsc --noEmit  [mem<=$LIMIT_MEM cpu<=$LIMIT_CPU wall<=${LIMIT_WALL}s nodeHeap=${NODE_HEAP}M] =="
  (cd "$ROOT/webui" && run_capped "$LIMIT_MEM" "$LIMIT_CPU" "$LIMIT_WALL" -- \
    env NODE_OPTIONS="--max-old-space-size=$NODE_HEAP" "$bin" --noEmit)
}

suite_smoke() {
  local bin="$ROOT/bin/pi-switch"
  require_exec "$bin"
  echo "== binary smoke  [mem<=$SMOKE_MEM cpu<=$SMOKE_CPU wall<=45s each] =="
  local c
  for c in "--help" "version" "build-info" "gateway status"; do
    echo "-- pi-switch $c"
    # shellcheck disable=SC2086
    run_capped "$SMOKE_MEM" "$SMOKE_CPU" 45 -- "$bin" $c
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
