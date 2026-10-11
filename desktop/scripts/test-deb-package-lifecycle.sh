#!/usr/bin/env bash
set -euo pipefail

if [[ "${GITHUB_ACTIONS:-}" != true ]]; then
  echo "run this package-manager test only on its disposable GitHub-hosted runner" >&2
  exit 2
fi

base_deb=$(realpath "${1:?usage: test-deb-package-lifecycle.sh package.deb}")
package_name=pi-switch-mygo-spike
install_path=/opt/pi-switch-mygo-spike/pi-switch-mygo-spike
launcher_path=/usr/bin/pi-switch-mygo-spike
architecture=$(dpkg-deb -f "$base_deb" Architecture)
base_version=$(dpkg-deb -f "$base_deb" Version)
runner_architecture=$(dpkg --print-architecture)

if [[ "$architecture" != "$runner_architecture" ]]; then
  echo "package architecture $architecture does not match runner $runner_architecture" >&2
  exit 1
fi
if dpkg-query -W "$package_name" >/dev/null 2>&1; then
  echo "$package_name is already registered before the isolated lifecycle test" >&2
  exit 1
fi

work_dir=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/pi-switch-deb-lifecycle.XXXXXX")

upgrade_version="${base_version}+ci1"
dpkg --compare-versions "$upgrade_version" gt "$base_version"
package_root="$work_dir/upgrade-package"
mkdir -p "$package_root"
dpkg-deb --raw-extract "$base_deb" "$package_root"
upgrade_marker=opt/pi-switch-mygo-spike/.ci-upgrade-marker
printf 'synthetic higher-version payload\n' >"$package_root/$upgrade_marker"
(
  cd "$package_root"
  find . -type f ! -path './DEBIAN/*' -print0 | sort -z | xargs -0r md5sum | sed 's#  \./#  #' >"$work_dir/md5sums"
)
mv "$work_dir/md5sums" "$package_root/DEBIAN/md5sums"
awk -v version="$upgrade_version" '
  BEGIN { count = 0 }
  /^Version:/ { print "Version: " version; count++; next }
  { print }
  END { if (count != 1) exit 1 }
' "$package_root/DEBIAN/control" >"$work_dir/control"
mv "$work_dir/control" "$package_root/DEBIAN/control"
upgrade_deb="$work_dir/${package_name}_${upgrade_version}_${architecture}.deb"
dpkg-deb --build --root-owner-group "$package_root" "$upgrade_deb"
test "$(dpkg-deb -f "$upgrade_deb" Version)" = "$upgrade_version"

user_home="$work_dir/user-home"
mkdir -p "$user_home/.pi-switch" "$user_home/.pi/agent/sessions"
printf '{"sentinel":"preserve"}\n' >"$user_home/.pi-switch/config.json"
printf '{"sentinel":"preserve"}\n' >"$user_home/.pi-switch/models.json"
printf 'test database sentinel\n' >"$user_home/.pi-switch/requests.db"
printf '{"type":"session","id":"sentinel"}\n' >"$user_home/.pi/agent/sessions/session.jsonl"
assert_user_data() {
  grep -qx '{"sentinel":"preserve"}' "$user_home/.pi-switch/config.json"
  grep -qx '{"sentinel":"preserve"}' "$user_home/.pi-switch/models.json"
  grep -qx 'test database sentinel' "$user_home/.pi-switch/requests.db"
  grep -qx '{"type":"session","id":"sentinel"}' "$user_home/.pi/agent/sessions/session.jsonl"
}
assert_version() {
  local expected=$1 actual
  actual=$(dpkg-query -W -f='${Version}' "$package_name")
  if [[ "$actual" != "$expected" ]]; then
    echo "installed version $actual, expected $expected" >&2
    exit 1
  fi
}

sudo apt-get update
sudo env HOME="$user_home" apt-get install -y --no-install-recommends "$base_deb"
assert_version "$base_version"
test -x "$install_path"
test -e "$launcher_path"
test ! -e "/$upgrade_marker"
dpkg --verify "$package_name"
assert_user_data

sudo env HOME="$user_home" apt-get install --reinstall -y --no-install-recommends "$base_deb"
assert_version "$base_version"
assert_user_data

sudo env HOME="$user_home" apt-get install -y --no-install-recommends "$upgrade_deb"
assert_version "$upgrade_version"
test -x "$install_path"
grep -qx 'synthetic higher-version payload' "/$upgrade_marker"
dpkg --verify "$package_name"
assert_user_data

sudo env HOME="$user_home" apt-get install --allow-downgrades -y --no-install-recommends "$base_deb"
assert_version "$base_version"
test -x "$install_path"
test ! -e "/$upgrade_marker"
dpkg --verify "$package_name"
assert_user_data

sudo env HOME="$user_home" apt-get remove -y "$package_name"
test ! -e "$install_path"
assert_user_data
sudo env HOME="$user_home" apt-get install -y --no-install-recommends "$base_deb"
assert_version "$base_version"
assert_user_data
sudo env HOME="$user_home" apt-get purge -y "$package_name"
test ! -e "$install_path"
test ! -e "$launcher_path"
if dpkg-query -W "$package_name" >/dev/null 2>&1; then
  echo "package registration remains after purge" >&2
  exit 1
fi
assert_user_data

echo "Debian package install/reinstall/changed-payload upgrade/downgrade/remove/purge and isolated user-data preservation passed"
