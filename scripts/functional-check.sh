#!/usr/bin/env bash
set -euo pipefail

binary=${1:-bin/pi-switch}
if [ ! -x "$binary" ]; then
  echo "functional check: binary is not executable: $binary" >&2
  exit 1
fi
workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT
export PI_SWITCH_CONFIG="$workdir/config.json"
export PI_SWITCH_DB="$workdir/requests.db"

version=$("$binary" --version)
info=$("$binary" --build-info)
node -e 'const info=JSON.parse(process.argv[1]); const version=process.argv[2]; if (!info.version || info.version !== version || !info.commit || !info.target || !info.buildTime) { console.error(info); process.exit(1); }' "$info" "$version"
"$binary" config path >/dev/null
printf 'functional smoke passed for %s (%s)\n' "$binary" "$version"
