#!/usr/bin/env bash
set -euo pipefail
# Cross-compile matrix linux/darwin/windows x amd64/arm64 (pure Go, -s -w)
# Output: bin/pi-switch-<goos>-<goarch>[.exe]

if [ ! -d "webui/dist" ] || [ -z "$(ls -A webui/dist 2>/dev/null)" ]; then
  echo "Building webui..."
  npm run build:webui
fi

mkdir -p bin
BUILD_FLAGS='-s -w'
for GOOS in linux darwin windows; do
  for GOARCH in amd64 arm64; do
    EXT=""
    if [ "$GOOS" = "windows" ]; then EXT=".exe"; fi
    OUT="bin/pi-switch-${GOOS}-${GOARCH}${EXT}"
    echo "Building $OUT ..."
    GOOS=$GOOS GOARCH=$GOARCH go build -ldflags "$BUILD_FLAGS" -o "$OUT" ./cmd/pi-switch
  done
done
echo "All builds done:"
ls -lh bin/pi-switch-* 2>/dev/null || true
if command -v npm >/dev/null 2>&1; then
  echo "npm pack dry-run:"
  npm pack --dry-run 2>&1 | grep -E "pi-switch-" || true
fi
