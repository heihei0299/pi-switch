#!/usr/bin/env bash
set -euo pipefail

binary=${1:-bin/pi-switch}
if [ ! -x "$binary" ]; then
  echo "functional check: binary is not executable: $binary" >&2
  exit 1
fi
workdir=$(mktemp -d)
server_pid=
proxy_started=false
cleanup() {
  if [ "$proxy_started" = true ]; then
    "$binary" proxy stop >/dev/null 2>&1 || true
  fi
  if [ -n "$server_pid" ]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -rf "$workdir"
}
trap cleanup EXIT
export HOME="$workdir/home"
export XDG_CONFIG_HOME="$workdir/xdg/config"
export XDG_DATA_HOME="$workdir/xdg/data"
export PI_AGENT_ROOT="$workdir/agent"
export PI_AGENT_SETTINGS="$workdir/agent/settings.json"
export PI_SWITCH_CONFIG_DIR="$workdir/config-dir"
export PI_SWITCH_CONFIG="$workdir/config.json"
export PI_SWITCH_DB="$workdir/requests.db"
export PI_SWITCH_MODELS="$workdir/models.json"

version=$("$binary" --version)
info=$("$binary" --build-info)
node -e 'const info=JSON.parse(process.argv[1]); const version=process.argv[2]; if (!info.version || info.version !== version || !info.commit || !info.target || !info.buildTime) { console.error(info); process.exit(1); }' "$info" "$version"
"$binary" config path >/dev/null
printf '%s\n' '{"ts":"2026-08-03T12:00:00Z","provider":"legacy-smoke","model":"compat-model","ok":true,"status":200,"promptTokens":13,"completionTokens":7,"cachedTokens":0,"reasoningTokens":0,"costTotal":0.002}' >"$workdir/requests.log"
legacy_log_hash=$(sha256sum "$workdir/requests.log" | cut -d' ' -f1)

port=$(node -e 'const net=require("node:net");const s=net.createServer();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})')
"$binary" webui start --host 127.0.0.1 --port "$port" >"$workdir/webui.log" 2>&1 &
server_pid=$!
ready=false
for _ in $(seq 1 50); do
  if curl -fsS -o /dev/null "http://127.0.0.1:$port/healthz" 2>/dev/null; then
    ready=true
    break
  fi
  sleep 0.1
done
if [ "$ready" != true ]; then
  cat "$workdir/webui.log" >&2
  echo "functional check: management API did not start" >&2
  exit 1
fi
root_status=$(curl -sS -o "$workdir/root" -w '%{http_code}' "http://127.0.0.1:$port/")
test "$root_status" = 410
curl -fsS "http://127.0.0.1:$port/api/buildInfo" >"$workdir/build-info.json"
node - "$workdir/build-info.json" <<'NODE'
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const info = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const index = fs.readFileSync('webui/dist/index.html');
const assets = fs.readdirSync('webui/dist/assets').sort();
let script = assets.find(name => name.startsWith('index-') && name.endsWith('.js'));
script ??= assets.find(name => name.endsWith('.js'));
if (!script && index.length) script = index.toString().match(/\/assets\/[^"' >]+\.js/)?.[0];
assert.equal(info.webui.embedded, index.length > 0);
assert.equal(info.webui.indexHash, crypto.createHash('sha256').update(index).digest('hex'));
assert.equal(info.webui.script, script ? (script.startsWith('/assets/') ? script : `/assets/${script}`) : '');
assert.equal(info.webui.assetCount, assets.length);
NODE
legacy_stats_ready=false
for _ in $(seq 1 50); do
  if curl -fsS "http://127.0.0.1:$port/api/stats" >"$workdir/legacy-stats.json" && node - "$workdir/legacy-stats.json" <<'NODE'
const fs = require('node:fs');
const stats = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
if (stats.totalRequests !== 1 || stats.okRequests !== 1 || stats.totalTokens.input !== 13 || stats.totalTokens.output !== 7) process.exit(1);
NODE
  then
    legacy_stats_ready=true
    break
  fi
  sleep 0.1
done
if [ "$legacy_stats_ready" != true ]; then
  cat "$workdir/legacy-stats.json" >&2
  echo "functional check: startup did not import the legacy requests.log into SQLite stats" >&2
  exit 1
fi
test "$(sha256sum "$workdir/requests.log" | cut -d' ' -f1)" = "$legacy_log_hash"
assets_status=$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port/assets/index.js")
test "$assets_status" = 404
kill "$server_pid"
wait "$server_pid" 2>/dev/null || true
server_pid=

proxy_port=$(node -e 'const net=require("node:net");const s=net.createServer();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})')
if ! "$binary" proxy start --daemon --host 127.0.0.1 --port "$proxy_port" >"$workdir/proxy-start.log" 2>&1; then
  cat "$workdir/proxy-start.log" >&2
  echo "functional check: managed Proxy did not start" >&2
  exit 1
fi
proxy_started=true
if ! "$binary" proxy status >"$workdir/proxy-status.log" 2>&1; then
  cat "$workdir/proxy-status.log" >&2
  echo "functional check: managed Proxy status did not verify" >&2
  exit 1
fi
proxy_ready=false
for _ in $(seq 1 50); do
  if curl -fsS -o /dev/null "http://127.0.0.1:$proxy_port/healthz" 2>/dev/null; then
    proxy_ready=true
    break
  fi
  sleep 0.1
done
if [ "$proxy_ready" != true ]; then
  cat "$workdir/config-dir/proxy.log" >&2
  echo "functional check: managed Proxy health endpoint did not become ready" >&2
  exit 1
fi
"$binary" proxy stop >"$workdir/proxy-stop.log" 2>&1
proxy_stopped=false
for _ in $(seq 1 50); do
  if ! curl -fsS -o /dev/null "http://127.0.0.1:$proxy_port/healthz" 2>/dev/null; then
    proxy_stopped=true
    break
  fi
  sleep 0.1
done
if [ "$proxy_stopped" != true ] || [ -e "$PI_SWITCH_CONFIG_DIR/proxy.pid" ]; then
  cat "$workdir/proxy-stop.log" >&2
  echo "functional check: managed Proxy listener remained after stop" >&2
  exit 1
fi
if "$binary" proxy status >"$workdir/proxy-stopped-status.log" 2>&1; then
  cat "$workdir/proxy-stopped-status.log" >&2
  echo "functional check: stopped Proxy still reports running" >&2
  exit 1
fi
proxy_started=false
printf 'functional smoke passed for %s (%s; API-only router, legacy SQLite import, and managed headless Proxy lifecycle)\n' "$binary" "$version"
