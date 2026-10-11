#!/usr/bin/env bash
set -euo pipefail

OUT=${1:-bin/pi-switch}
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"

TARGET_OS=${GOOS:-$(go env GOOS)}
TARGET_ARCH=${GOARCH:-$(go env GOARCH)}
if [[ "$TARGET_OS" != linux || ( "$TARGET_ARCH" != amd64 && "$TARGET_ARCH" != arm64 ) ]]; then
  echo "Unsupported build target: ${TARGET_OS}/${TARGET_ARCH} (supported: linux/amd64, linux/arm64)" >&2
  exit 1
fi

mkdir -p "$(dirname "$OUT")"

VER=$(node -p "JSON.parse(require('fs').readFileSync('package.json','utf8')).version")
COMMIT=$(git rev-parse --short=7 HEAD 2>/dev/null || printf 'unknown')
if git diff --quiet -- . ':!bin'; then
  DIRTY=false
else
  DIRTY=true
fi
if [ -n "${SOURCE_DATE_EPOCH:-}" ]; then
  BUILD_TIME=$(date -u -d "@${SOURCE_DATE_EPOCH}" +%Y-%m-%dT%H:%M:%SZ)
else
  BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
fi
TARGET="${TARGET_OS}/${TARGET_ARCH}"
LDFLAGS="-s -w"
LDFLAGS+=" -X main.version=${VER}"
LDFLAGS+=" -X main.buildTime=${BUILD_TIME}"
LDFLAGS+=" -X main.buildCommit=${COMMIT}"
LDFLAGS+=" -X main.buildTarget=${TARGET}"
LDFLAGS+=" -X main.buildDirty=${DIRTY}"
LDFLAGS+=" -X github.com/heihei0299/pi-switch/internal/server.Version=${VER}"
LDFLAGS+=" -X github.com/heihei0299/pi-switch/internal/server.BuildTime=${BUILD_TIME}"
LDFLAGS+=" -X github.com/heihei0299/pi-switch/internal/server.BuildCommit=${COMMIT}"
LDFLAGS+=" -X github.com/heihei0299/pi-switch/internal/server.BuildTarget=${TARGET}"
LDFLAGS+=" -X github.com/heihei0299/pi-switch/internal/server.BuildDirty=${DIRTY}"
mapfile -t WEBUI_BUILD_INFO < <(node <<'NODE'
const crypto = require('node:crypto');
const fs = require('node:fs');
const indexPath = 'webui/dist/index.html';
const placeholder = '<!doctype html><html><head><meta charset="utf-8"><title>pi-switch</title></head><body><div id="root">pi-switch WebUI placeholder</div><p>embed.FS placeholder for webui/dist</p></body></html>';
const index = fs.existsSync(indexPath) ? fs.readFileSync(indexPath) : Buffer.alloc(0);
const assetsDir = 'webui/dist/assets';
const assets = fs.existsSync(assetsDir) ? fs.readdirSync(assetsDir).sort() : [];
let script = assets.find(name => name.startsWith('index-') && name.endsWith('.js'));
script ??= assets.find(name => name.endsWith('.js'));
if (!script && index.length) script = index.toString().match(/\/assets\/[^"' >]+\.js/)?.[0];
console.log(index.length > 0);
console.log(crypto.createHash('sha256').update(index.length ? index : placeholder).digest('hex'));
console.log(script ? (script.startsWith('/assets/') ? script : `/assets/${script}`) : '');
console.log(assets.length);
NODE
)
LDFLAGS+=" -X github.com/heihei0299/pi-switch/internal/server.webuiEmbeddedBuild=${WEBUI_BUILD_INFO[0]}"
LDFLAGS+=" -X github.com/heihei0299/pi-switch/internal/server.webuiIndexHashBuild=${WEBUI_BUILD_INFO[1]}"
LDFLAGS+=" -X github.com/heihei0299/pi-switch/internal/server.webuiScriptBuild=${WEBUI_BUILD_INFO[2]}"
LDFLAGS+=" -X github.com/heihei0299/pi-switch/internal/server.webuiAssetCountBuild=${WEBUI_BUILD_INFO[3]}"

# Serial Package Build: -p 1 keeps one compile job in flight so a build on a
# shared machine does not saturate every core. GO_MAXPROCS caps the toolchain's
# own runtime parallelism; both can be raised explicitly when a fast build is
# wanted (GO_MAXPROCS=0 means "use the default").
GO_MAXPROCS=${GO_MAXPROCS:-2}
GO_BUILD_PARALLEL=${GO_BUILD_PARALLEL:-1}

env GOOS="${TARGET_OS}" GOARCH="${TARGET_ARCH}" GOMAXPROCS="${GO_MAXPROCS}" \
  go build -p "${GO_BUILD_PARALLEL}" -ldflags "$LDFLAGS" -o "$OUT" ./cmd/pi-switch
