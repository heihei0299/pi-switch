#!/usr/bin/env bash
set -euo pipefail

binary=${1:-bin/pi-switch}
if [ ! -x "$binary" ]; then
  echo "functional check: binary is not executable: $binary" >&2
  exit 1
fi
workdir=$(mktemp -d)
server_pid=
cleanup() {
  if [ -n "$server_pid" ]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -rf "$workdir"
}
trap cleanup EXIT
export PI_SWITCH_CONFIG="$workdir/config.json"
export PI_SWITCH_DB="$workdir/requests.db"

version=$("$binary" --version)
info=$("$binary" --build-info)
node -e 'const info=JSON.parse(process.argv[1]); const version=process.argv[2]; if (!info.version || info.version !== version || !info.commit || !info.target || !info.buildTime) { console.error(info); process.exit(1); }' "$info" "$version"
"$binary" config path >/dev/null

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
assets_status=$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:$port/assets/index.js")
test "$assets_status" = 404
kill "$server_pid"
wait "$server_pid" 2>/dev/null || true
server_pid=
printf 'functional smoke passed for %s (%s; API-only WebUI router)\n' "$binary" "$version"
