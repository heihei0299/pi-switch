#!/usr/bin/env bash
set -euo pipefail

OUT=${1:-bin/pi-switch}
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
mkdir -p "$(dirname "$OUT")"

VER=$(node -p "JSON.parse(require('fs').readFileSync('package.json','utf8')).version")
COMMIT=$(git rev-parse HEAD 2>/dev/null || printf 'unknown')
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
TARGET="${GOOS:-$(go env GOOS)}/${GOARCH:-$(go env GOARCH)}"
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

env GOOS="${GOOS:-$(go env GOOS)}" GOARCH="${GOARCH:-$(go env GOARCH)}" go build -ldflags "$LDFLAGS" -o "$OUT" ./cmd/pi-switch
