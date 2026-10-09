#!/usr/bin/env bash
set -euo pipefail
# Cross-compile Linux and macOS for amd64/arm64 (pure Go, -s -w)
# Output: bin/pi-switch-<goos>-<goarch>

if [ ! -d "webui/dist" ] || [ -z "$(ls -A webui/dist 2>/dev/null)" ]; then
  echo "Building webui..."
  npm run build:webui
fi

mkdir -p bin
for GOOS in linux darwin; do
  for GOARCH in amd64 arm64; do
    OUT="bin/pi-switch-${GOOS}-${GOARCH}"
    echo "Building $OUT ..."
    GOOS=$GOOS GOARCH=$GOARCH bash scripts/build-go.sh "$OUT"
  done
done
echo "All builds done:"
ls -lh bin/pi-switch-* 2>/dev/null || true
if command -v npm >/dev/null 2>&1; then
  echo "npm pack dry-run:"
  npm pack --dry-run 2>&1 | grep -E "pi-switch-" || true
fi
