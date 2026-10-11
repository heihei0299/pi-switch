#!/usr/bin/env bash
set -euo pipefail

if [[ "${GITHUB_ACTIONS:-}" != true ]]; then
  echo "run this package-manager test only on its disposable GitHub-hosted runner" >&2
  exit 2
fi

current_deb=$(realpath "${1:?usage: test-deb-package-lifecycle.sh current.deb [previous.deb]}")
previous_deb=${2:-}
package_name=pi-switch-mygo-spike
install_path=/opt/pi-switch-mygo-spike/pi-switch-mygo-spike
launcher_path=/usr/bin/pi-switch-mygo-spike
architecture=$(dpkg-deb -f "$current_deb" Architecture)
current_version=$(dpkg-deb -f "$current_deb" Version)
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
current_root="$work_dir/current-root"
mkdir -p "$current_root"
dpkg-deb -x "$current_deb" "$current_root"
current_app="$current_root$install_path"
current_app_sha=$(sha256sum "$current_app" | awk '{print $1}')
test -x "$current_app"

if [[ -n "$previous_deb" ]]; then
  previous_deb=$(realpath "$previous_deb")
  previous_architecture=$(dpkg-deb -f "$previous_deb" Architecture)
  previous_package=$(dpkg-deb -f "$previous_deb" Package)
  if [[ "$previous_architecture" != "$architecture" || "$previous_package" != "$package_name" ]]; then
    echo "previous package identity does not match current package" >&2
    exit 1
  fi

  previous_version="${current_version}~previous"
  dpkg --compare-versions "$previous_version" lt "$current_version"
  package_root="$work_dir/previous-package"
  mkdir -p "$package_root"
  dpkg-deb --raw-extract "$previous_deb" "$package_root"
  awk -v version="$previous_version" '
    BEGIN { count = 0 }
    /^Version:/ { print "Version: " version; count++; next }
    { print }
    END { if (count != 1) exit 1 }
  ' "$package_root/DEBIAN/control" >"$work_dir/control"
  mv "$work_dir/control" "$package_root/DEBIAN/control"
  previous_test_deb="$work_dir/${package_name}_${previous_version}_${architecture}.deb"
  dpkg-deb --build --root-owner-group "$package_root" "$previous_test_deb"
  test "$(dpkg-deb -f "$previous_test_deb" Version)" = "$previous_version"

  previous_root="$work_dir/previous-root"
  mkdir -p "$previous_root"
  dpkg-deb -x "$previous_test_deb" "$previous_root"
  previous_app="$previous_root$install_path"
  previous_app_sha=$(sha256sum "$previous_app" | awk '{print $1}')
  if [[ "$current_app_sha" == "$previous_app_sha" ]]; then
    echo "previous and current packages contain the same desktop executable" >&2
    exit 1
  fi
fi

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
if [[ -n "$previous_deb" ]]; then
  sudo env HOME="$user_home" apt-get install -y --no-install-recommends "$previous_test_deb"
  assert_version "$previous_version"
  test -x "$install_path"
  test "$(sha256sum "$install_path" | awk '{print $1}')" = "$previous_app_sha"
  dpkg --verify "$package_name"
  assert_user_data

  sudo env HOME="$user_home" apt-get install -y --no-install-recommends "$current_deb"
  assert_version "$current_version"
  test "$(sha256sum "$install_path" | awk '{print $1}')" = "$current_app_sha"
  dpkg --verify "$package_name"
  assert_user_data

  sudo env HOME="$user_home" apt-get install --allow-downgrades -y --no-install-recommends "$previous_test_deb"
  assert_version "$previous_version"
  test "$(sha256sum "$install_path" | awk '{print $1}')" = "$previous_app_sha"
  dpkg --verify "$package_name"
  assert_user_data
fi

sudo env HOME="$user_home" apt-get install -y --no-install-recommends "$current_deb"
assert_version "$current_version"
test -x "$install_path"
test -e "$launcher_path"
test "$(sha256sum "$install_path" | awk '{print $1}')" = "$current_app_sha"
dpkg --verify "$package_name"
sudo env HOME="$user_home" apt-get install --reinstall -y --no-install-recommends "$current_deb"
assert_version "$current_version"
test "$(sha256sum "$install_path" | awk '{print $1}')" = "$current_app_sha"
dpkg --verify "$package_name"
assert_user_data

sudo env HOME="$user_home" apt-get remove -y "$package_name"
test ! -e "$install_path"
assert_user_data
sudo env HOME="$user_home" apt-get install -y --no-install-recommends "$current_deb"
assert_version "$current_version"
test "$(sha256sum "$install_path" | awk '{print $1}')" = "$current_app_sha"
dpkg --verify "$package_name"
assert_user_data
sudo env HOME="$user_home" apt-get purge -y "$package_name"
test ! -e "$install_path"
test ! -e "$launcher_path"
if dpkg-query -W "$package_name" >/dev/null 2>&1; then
  echo "package registration remains after purge" >&2
  exit 1
fi
assert_user_data

echo "Debian package install/reinstall/remove/purge and isolated user-data preservation passed"
if [[ -n "$previous_deb" ]]; then
  echo "previous/current app-binary upgrade/downgrade passed (previous=$previous_app_sha current=$current_app_sha)"
fi
