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
cat >"$PI_SWITCH_CONFIG" <<'JSON'
{"version":1,"profiles":{},"settings":{"providerPrefix":"pi-switch","writeMode":"gateway","gatewayApi":"openai-completions","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110},"injectOpenCodeAttribution":true}}
JSON
chmod 0600 "$PI_SWITCH_CONFIG"
legacy_config_hash=$(sha256sum "$PI_SWITCH_CONFIG" | cut -d' ' -f1)
python3 - "$PI_SWITCH_DB" <<'PY'
import sqlite3, sys

db = sqlite3.connect(sys.argv[1])
db.execute("""CREATE TABLE requests (
  id INTEGER PRIMARY KEY AUTOINCREMENT, ts TEXT, provider TEXT, model TEXT, success INTEGER,
  prompt_tokens INTEGER, completion_tokens INTEGER, cached_tokens INTEGER, conversation_id TEXT
)""")
db.execute("""INSERT INTO requests(ts, provider, model, success, prompt_tokens, completion_tokens, cached_tokens, conversation_id)
  VALUES (?, ?, ?, ?, ?, ?, ?, ?)""",
  ("2026-08-02T10:00:00Z", "legacy-db", "old-model", 0, 100, 20, 0, None))
db.commit()
db.close()
PY
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
curl -fsS "http://127.0.0.1:$port/api/config" >"$workdir/legacy-config-response.json"
node - "$workdir/legacy-config-response.json" "$workdir/migrated-config.json" <<'NODE'
const assert = require('node:assert/strict');
const fs = require('node:fs');
const response = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
assert.equal(response.config.version, 2);
assert.equal(response.config.settings.conversationSource, 'proxy');
assert.equal(Object.hasOwn(response.config.settings, 'injectOpenCodeAttribution'), false);
assert.equal(response.config.settings.proxy.port, 43112);
fs.writeFileSync(process.argv[3], JSON.stringify(response.config));
NODE
test "$(sha256sum "$PI_SWITCH_CONFIG" | cut -d' ' -f1)" = "$legacy_config_hash"
curl -fsS -X PUT -H 'Content-Type: application/json' --data-binary "@$workdir/migrated-config.json" "http://127.0.0.1:$port/api/config" >/dev/null
node - "$PI_SWITCH_CONFIG" <<'NODE'
const assert = require('node:assert/strict');
const fs = require('node:fs');
const config = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
assert.equal(config.version, 2);
assert.equal(config.settings.conversationSource, 'proxy');
assert.equal(Object.hasOwn(config.settings, 'injectOpenCodeAttribution'), false);
assert.equal(config.settings.proxy.port, 43112);
NODE
test "$(stat -c '%a' "$PI_SWITCH_CONFIG")" = 600
root_status=$(curl -sS -o "$workdir/root" -w '%{http_code}' "http://127.0.0.1:$port/")
test "$root_status" = 410
grep -q "WebUI has been retired" "$workdir/root"
curl -fsS "http://127.0.0.1:$port/api/buildInfo" >"$workdir/build-info.json"
node - "$workdir/build-info.json" "$version" <<'NODE'
const assert = require('node:assert/strict');
const fs = require('node:fs');
const info = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
assert.equal(info.version, process.argv[3]);
assert.equal(info.status, 'ok');
assert.deepEqual(info.webui, { embedded: false, indexHash: '', script: '', assetCount: 0 });
NODE
legacy_stats_ready=false
for _ in $(seq 1 50); do
  if curl -fsS "http://127.0.0.1:$port/api/stats" >"$workdir/legacy-stats.json" && node - "$workdir/legacy-stats.json" <<'NODE'
const fs = require('node:fs');
const stats = JSON.parse(fs.readFileSync(process.argv[2], 'utf8'));
const oldProvider = stats.byProvider['legacy-db'];
const importedProvider = stats.byProvider['legacy-smoke'];
const oldRequest = stats.recentRequests.find(request => request.provider === 'legacy-db');
if (stats.totalRequests !== 2 || stats.okRequests !== 1 || stats.failedRequests !== 1 || stats.totalTokens.input !== 13 || stats.totalTokens.output !== 7) process.exit(1);
if (!oldProvider || oldProvider.total !== 1 || oldProvider.failed !== 1 || !importedProvider || importedProvider.ok !== 1) process.exit(1);
if (!oldRequest || oldRequest.success !== false || oldRequest.promptTokens !== 100 || oldRequest.completionTokens !== 20 || oldRequest.status !== null || oldRequest.error !== null || oldRequest.upstream_url !== null) process.exit(1);
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
printf 'functional smoke passed for %s (%s; API-only router, legacy config/SQLite schema/log migration, and managed headless Proxy lifecycle)\n' "$binary" "$version"
