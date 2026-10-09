#!/usr/bin/env bash
set -euo pipefail
# Cross-compile Linux for amd64/arm64 (pure Go, -s -w)
# Output: bin/pi-switch-<goos>-<goarch>

if [ ! -d "webui/dist" ] || [ -z "$(ls -A webui/dist 2>/dev/null)" ]; then
  echo "Building webui..."
  npm run build:webui
fi

mkdir -p bin
for GOARCH in amd64 arm64; do
  OUT="bin/pi-switch-linux-${GOARCH}"
  echo "Building $OUT ..."
  GOOS=linux GOARCH=$GOARCH bash scripts/build-go.sh "$OUT"
done
echo "All builds done:"
ls -lh bin/pi-switch-* 2>/dev/null || true
if command -v npm >/dev/null 2>&1; then
  echo "npm pack dry-run:"
  npm pack --dry-run 2>&1 | grep -E "pi-switch-" || true
fi
