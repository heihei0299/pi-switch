---
name: container-functional-test
description: Run functional tests in an incus Ubuntu container with strict concurrency and resource limits. Use when verifying pi-switch features end-to-end in an isolated environment without polluting the host.
---

# Container Functional Test (Incus + Limited Resources)

Isolated functional verification for pi-switch that mirrors `cc-switch`'s supplier-side validation, but with hard resource caps. The container's filesystem and `HOME` are fully isolated from the host's `~/.pi` and `~/.pi-switch`.

## When to use

- Need to verify `spec` coverage end-to-end (e.g., `pi-session-supplier-scan`'s `8+3` cases: `F1-F8/B1-B3`) without touching host's `requests.log` or `sessions`.
- Need to reproduce a bug under constrained resources (2c/2G) to catch `MAX_TREE_ENTRIES`/`128M` leaks.
- Need to prove `cargo` + `webui` remain green after a Rust change when the host lacks a toolchain.

## Prerequisites

- Host: Arch Linux with `incus` 7.3 (`sudo pacman -S incus && sudo systemctl enable --now incus && incus admin init --minimal`).
- Image: `images:ubuntu/noble` (24.04, 133M container, 160M cloud). `jammy` (22.04) also works.
- Repo: current `dev` branch with `webui/dist` buildable via `npm run build:webui`.

## One-shot flow

```bash
# 1. Launch with hard caps
incus launch images:ubuntu/noble pi-switch-ft \
  --config limits.cpu=2 --config limits.memory=2GiB \
  --config security.nesting=true
# wait 3s for network
incus exec pi-switch-ft -- bash -lc 'apt-get update && apt-get install -y curl git build-essential pkg-config libssl-dev'

# 2. Toolchains (pinned)
incus exec pi-switch-ft -- bash -lc 'curl -fsSL https://deb.nodesource.com/setup_23.x | bash - && apt-get install -y nodejs && node -v'
incus exec pi-switch-ft -- bash -lc 'curl --proto "=https" --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y && source $HOME/.cargo/env && cargo --version'

# 3. Ship the repo (host -> container, no git history needed)
tar czf /tmp/pi-switch.tar.gz --exclude='.git' --exclude='node_modules' --exclude='target' --exclude='webui/node_modules' --exclude='webui/dist' -C /home/shial/Project pi-switch
incus file push /tmp/pi-switch.tar.gz pi-switch-ft/root/
incus exec pi-switch-ft -- bash -lc 'mkdir -p /root/pi-switch && tar xzf /root/pi-switch.tar.gz -C /root/pi-switch --strip-components=1'

# 4. Ship the built web assets (or build inside)
tar czf /tmp/dist.tar.gz -C /home/shial/Project/pi-switch/webui dist
incus file push /tmp/dist.tar.gz pi-switch-ft/root/
incus exec pi-switch-ft -- bash -lc 'mkdir -p /root/pi-switch/webui && tar xzf /root/dist.tar.gz -C /root/pi-switch/webui'

# 5. Install JS deps with low priority
incus exec pi-switch-ft -- bash -lc 'cd /root/pi-switch && nice -n 10 npm ci --prefer-offline'
incus exec pi-switch-ft -- bash -lc 'cd /root/pi-switch && nice -n 10 npm --prefix webui install'

# 6. Tests with limited concurrency
incus exec pi-switch-ft -- bash -lc 'source $HOME/.cargo/env; cd /root/pi-switch; nice -n 10 cargo test -- --test-threads=2'
incus exec pi-switch-ft -- bash -lc 'cd /root/pi-switch; nice -n 10 env NODE_ENV=test UV_THREADPOOL_SIZE=2 NODE_OPTIONS="--max-old-space-size=1024" npm --prefix webui run test'

# 7. Cleanup
incus stop pi-switch-ft --force && incus delete pi-switch-ft --force
rm -f /tmp/pi-switch.tar.gz /tmp/dist.tar.gz
```

## Resource discipline

- Container: `limits.cpu=2` `limits.memory=2GiB` (incus cgroup hard cap, `free -h` should show `2.0Gi`).
- Host: `nice -n 10` `ionice -c2 -n6` for all `npm`/`cargo` invocations, `UV_THREADPOOL_SIZE=2`, `NODE_OPTIONS=--max-old-space-size=1024`, `cargo --test-threads=2`.
- Do not use `cargo test` without `--test-threads=2` (fails under 2G) or `npm` without `nice` (starves the host's `pi` TUI).

## What to assert

- `cargo`: `203 passed` (scan_pi 18 + stats 7 + config 10 + tui 12 + usage 30 + web 10 + ...). Known flake: `settings_get_and_put_roundtrip` needs `tmp/.pi/agent` pre-created before `PUT`.
- `webui`: `92 passed` (`StatsPanel 51 + format 26 + SettingsPanel 5 + conversationSource 3 + statsWindow 7`).
- Functional mapping: `F1 sessionScan / F2 ProjectDirectories / F3 proxy / F4 off / F5 off-hide / F6 migration / F7 unlabeled / F8 no-session / B1 ±2s / B2 prompt / B3 5min` are covered by `scan_pi` + `stats` + `web` suites above.

## Troubleshooting

- `webui/dist` missing (`RustEmbed`): push `dist.tar.gz` as in step 4, or `npm --prefix webui run build` inside.
- `msg` helper `control chars` → `serde_json::to_string(text).unwrap()` (raw `\x01` breaks `serde_json`).
- `scan_pi` off-by-one (`500k`): count `rest.len()` not `rest.len()+1`, `line_count > MAX+1`.
- `settings` 400: ensure `gatewayApi` in payload and `tmp/.pi/agent` exists before `PUT`.
