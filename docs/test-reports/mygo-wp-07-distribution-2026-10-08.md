# WP-07 Distribution progress — 2026-10-08

**Status: IN PROGRESS / Gate D NOT PASSED.** A separate CI path produces test artifacts only; nothing is published.

## Implemented

- Added a desktop-only GitHub Actions workflow for Linux amd64/arm64, Windows amd64/arm64 and macOS amd64/arm64.
- The workflow bundles a matching CLI executable, verifies its embedded commit/target against the checkout/matrix, records the same identity in a manifest, builds MyGo packages, scans source/package output, and uploads seven-day Actions artifacts only.
- Added a Linux amd64 package-job smoke that rejects an absolute-symlink archive without replacing the existing app or writing outside staging, then installs/reinstalls/uninstalls the real tarball under a temporary `HOME` and checks stale-file replacement plus preservation of config/models/requests sentinels and a user-owned same-name command.
- Kept the existing CLI/npm workflow unchanged. No release upload, npm publish, signing credentials, notarization credentials, or updater channel were added.
- Added Linux package metadata and documented GTK 3/WebKitGTK, optional AppIndicator fallback, WebView2, and the test-only signing status.

## Verification

| Check | Result |
| --- | --- |
| Desktop GitHub Actions YAML parse (`gopkg.in/yaml.v3`) | PASS |
| CLI build identity check (`strings` + exact-line `grep`) against local Go CLI builds for all six workflow targets | PASS; each binary embedded commit `2c2cc35` and the matching GOOS/GOARCH target (cross-build identity only, not GUI validation) |
| Current-source build identity matrix with `scripts/build-go.sh` for all six targets; Linux amd64 `--build-info` and `scripts/functional-check.sh` after plan-commit consolidation | PASS; all binaries embed commit `41126c4` and the expected target. The Linux smoke reports `dirty=true` because the worktree has unrelated tracked edits; this is local verification, not a clean release build or GUI validation. |
| `GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 go test -mod=readonly ./...` (`desktop/`) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 go vet -mod=readonly ./...` (`desktop/`) | PASS |
| `./node_modules/.bin/mygo vet .` (`desktop/`) | PASS |
| `GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 go test -mod=readonly ./internal/daemon` | PASS |
| `git diff --check` | PASS |
| Desktop Actions workflow YAML parse (`gopkg.in/yaml.v3`) | PASS (syntax only; no GitHub Actions schema linter installed) |
| `bash scripts/check-secrets.sh` in a clean isolated worktree | PASS |
| WebUI and desktop frontend production builds in an isolated worktree | PASS |
| Matching Linux CLI resource `--build-info` | PASS: commit `ff19a60`, target `linux/amd64`, `dirty=false` |
| `mygo build -platform linux/amd64 -skip-build-command -skip-notarize -o /tmp/pi-switch-wp07.uFfPO5/artifacts` | PASS; produced the app, `.deb`, tarball, install script and bundled CLI resource |
| Linux tarball `install.sh` install/reinstall/uninstall in a temporary `HOME`; verified replacement of stale app files and preservation of sentinel config/models/requests files | PASS; test home `/tmp/pi-switch-install-test.nTvOTq` |
| Linux tarball `install.sh` replacement from build `c499566` to test artifact `ff19a60` in a temporary `HOME`; verified app binary SHA changed, stale app file disappeared, and config/models/requests sentinels remained unchanged | PASS; test home `/tmp/pi-switch-wp07-upgrade.4moLCy/home`; both artifacts are labeled `0.1.0`, so this verifies replacement behavior, not a semantic-version upgrade |
| Built a test-only `0.1.1` tarball from an extracted source copy, then used its installer to upgrade the isolated `0.1.0` package; verified binary SHA changed, stale `0.1.0` file disappeared, and config/models/requests sentinels remained unchanged | PASS; test root `/tmp/pi-switch-wp07-semver.Ox98cv`; version bump was confined to the temporary source copy, and no release was published |
| Linux tarball `install.sh` install/uninstall with a pre-existing user-owned command at the app's command path | PASS; install left the command alone and uninstall preserved it; test home `/tmp/pi-switch-install-conflict.1E9M13` |
| `sh -n desktop/build/linux-amd64/install.sh` | PASS |
| `bash desktop/scripts/test-linux-installer.sh desktop/build/linux-amd64/install.sh desktop/build/linux-amd64/pi-switch-mygo-spike-0.1.0-linux-amd64.tar.gz` | PASS; rejected a crafted absolute-symlink archive without altering the prior install or escaping staging, then installed/reinstalled/uninstalled in a temporary `HOME` and preserved isolated config/models/requests sentinels and a user-owned same-name command |
| Static inspection of `desktop/build/linux-amd64/pi-switch-mygo-spike_0.1.0_amd64.deb` using `ar`/`tar` | PASS: valid `debian-binary`, control and data members; package `pi-switch-mygo-spike` `0.1.0` `amd64`; declares GTK 3/WebKitGTK dependencies and contains the app binary, desktop entry and icons |
| Credential-shaped filename scan of package output | PASS; no matches |

The first isolated packaging attempt caught that Go's embedded WebUI requires `webui/dist` before building the CLI resource. The workflow now builds both frontends first, then compiles the CLI and invokes MyGo with `-skip-build-command`; the corrected sequence passed in the isolated worktree. Existing generated outputs in the user's worktree were preserved.

## Not verified / remaining

- The GitHub workflow has not run remotely. Current local `mygo` contains `.github/workflows/desktop.yml`, but fetched `origin/mygo` (`5d6d823`) does not; `gh run list --workflow desktop.yml --branch mygo` returned 404 because the workflow is also absent from the default branch. No push/PR was created, so this workflow cannot yet be validated on GitHub.
- No published-release upgrade/update-channel migration, Debian package-manager transaction/rollback, Windows/macOS install or GUI launch, or daemon-lifecycle test was run. The `0.1.0` → `0.1.1` package test used a temporary source copy and user home; it is not a clean-system or native-window test.
- `dpkg` and `dpkg-deb` are unavailable on this host, so `.deb` verification was archive/metadata inspection only; no package-manager install/upgrade/rollback is claimed.
- The host lacks WebKitGTK, so the installer's missing-library diagnostic appeared as expected; the installed GUI was not launched.
- No release signing/notarization or update channel is configured. Artifacts are not suitable for public distribution.
- Full repository tests remain deferred until WP-05 through WP-08 are complete.
