#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
	echo "usage: $0 INSTALLER ARCHIVE" >&2
	exit 2
fi

installer=$1
archive=$2
test_root=$(mktemp -d "${TMPDIR:-/tmp}/pi-switch-installer.XXXXXX")
trap 'rm -rf -- "$test_root"' EXIT

export HOME="$test_root/home"
export XDG_DATA_HOME="$test_root/data"
export XDG_CONFIG_HOME="$HOME/.config"
export PI_SWITCH_CONFIG_DIR="$HOME/.pi-switch"
export PI_SWITCH_CONFIG="$PI_SWITCH_CONFIG_DIR/config.json"
export PI_SWITCH_DB="$PI_SWITCH_CONFIG_DIR/requests.db"
export PI_SWITCH_MODELS="$HOME/.pi/agent/models.json"

name=pi-switch-mygo-spike
app_dir="$HOME/.local/$name.app"
user_command="$HOME/.local/bin/$name"
config="$PI_SWITCH_CONFIG"
models="$PI_SWITCH_MODELS"
requests="$PI_SWITCH_DB"

mkdir -p "$app_dir" "$(dirname "$user_command")" "$(dirname "$config")" "$(dirname "$models")"
printf 'old version\n' >"$app_dir/stale.marker"
printf 'user-owned command\n' >"$user_command"
printf 'config sentinel\n' >"$config"
printf '{"models":"sentinel"}\n' >"$models"
printf 'requests sentinel\n' >"$requests"

outside="$test_root/outside"
malicious_archive="$test_root/malicious.tar.gz"
mkdir -p "$outside"
python3 - "$malicious_archive" "$outside" "$name" <<'PY'
import io
import sys
import tarfile

archive, outside, name = sys.argv[1:]
with tarfile.open(archive, "w:gz") as bundle:
    binary = tarfile.TarInfo(name)
    binary.mode = 0o755
    content = b"#!/bin/sh\nexit 0\n"
    binary.size = len(content)
    bundle.addfile(binary, io.BytesIO(content))
    link = tarfile.TarInfo("escape")
    link.type = tarfile.SYMTYPE
    link.linkname = outside
    bundle.addfile(link)
    payload = tarfile.TarInfo("escape/payload")
    content = b"must stay inside staging"
    payload.size = len(content)
    bundle.addfile(payload, io.BytesIO(content))
PY
if sh "$installer" "$malicious_archive" >"$test_root/malicious.log" 2>&1; then
	echo "installer accepted an archive that writes through an absolute symlink" >&2
	exit 1
fi
test -e "$app_dir/stale.marker"
test ! -e "$outside/payload"

sh "$installer" "$archive"
test -x "$app_dir/$name"
test ! -e "$app_dir/stale.marker"

printf 'stale after install\n' >"$app_dir/stale.marker"
sh "$installer" "$archive"
test -x "$app_dir/$name"
test ! -e "$app_dir/stale.marker"

sh "$installer" --uninstall
test ! -e "$app_dir"
grep -qx 'user-owned command' "$user_command"
grep -qx 'config sentinel' "$config"
grep -qx '{"models":"sentinel"}' "$models"
grep -qx 'requests sentinel' "$requests"

echo "Linux installer install/reinstall/uninstall and data-preservation smoke passed"
