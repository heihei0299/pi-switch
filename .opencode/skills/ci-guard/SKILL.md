---
name: ci-guard
description: "Guard the GitHub Actions release pipeline: orchestrate workflow flow, enforce pre-release verification, and self-correct after publish. Use when CI is flaky/failing, when setting up or editing .github/workflows/ci.yml, or before tagging a release to npm."
---

# CI Guard

**guard** 为领衔词的发布门禁技能：以一次**可复现的失败**为起点，把 `verify → build → publish` 编排成不可绕过的门，把**预发布校验**做成硬门槛，把**发布后自纠**做成闭环。本技能沉淀自 `heihei0299/pi-switch` 23 次运行中 14 次失败的复盘（见 `.scratch/research/ci-actions-调研.md`）——不替代 `diagnose-fix` 的通用诊断，只收敛 CI/发布这一条链。

## 何时用

- Actions 持续红 / 偶发红（尤其是 `verify` 单点红而 `publish` 仍绿）
- 新建或改动 `.github/workflows/ci.yml`、调整 `cargo test` / `clippy` / `rustfmt` 参数
- 打 tag 前、发 npm 前、或发布后需要自检/回滚

## 三段式门禁

```
① 编排 flows → ② 预发布 gate → ③ 发布后自纠
```

每段有**完成条件**（可验证），未满足不进入下一段。

---

### ① 编排 flows —— 让工作流不可被绕过

**做**：
- `on`：`push.tags: ["v*"]` **必须**同时配 `push.branches: [main]`（或 `master`）+ `pull_request.branches: [main]` + `workflow_dispatch`。否则直推 `main` 的修复（如 `2d68f62`）无法被 CI 验证，tag 才暴露问题
- `jobs` 依赖：`publish.needs: [build, verify]`，**禁止** `needs: build` 单依赖。门禁失效的直接原因就是 `verify` 红仍发包
- `permissions` 最小化：`verify`/`build` 只需 `contents: read`，仅 `publish` 保留 `contents: write` + `packages: write`（或 `id-token: write` 若用 OIDC）
- `concurrency`：`group: ci-${{ github.ref }}` + `cancel-in-progress: true`，避免同分支并行互踩
- `cache`：`rust-cache` 或 `actions/cache` 缓存 `~/.cargo` + `target`，`actions/setup-node` 加 `cache: npm`，避免每次 `npm install` 重装
- `find changed Rust files`：`git diff origin/main...HEAD` 在 tag 事件下为空，改为 `git diff --name-only HEAD~1...HEAD` 或直接全量 `cargo fmt --check` / `clippy`，避免误跳过

**完成条件**：
- [ ] `git diff HEAD -- .github/workflows/ci.yml` 显示 `on.push.branches` 存在
- [ ] `publish.needs` 包含 `verify`
- [ ] `workflow_dispatch` 可手动触发全量

---

### ② 预发布 gate —— 在写盘之前变红

本段是**硬门槛**，顺序固定：`fmt → clippy → test → build`，任一步红即阻断 `publish`。

**fmt / clippy**：
- `rustfmt --check` 与 `cargo clippy --all-targets -- -D warnings` 必须与本地一致（`rust-toolchain.toml` 锁定 `stable` 版本）
- 允许的 `-A` 必须显式列出（如本仓 `-A clippy::manual_checked_ops` 等 4 项），不批量 `-A clippy::all`

**test（关键）**：
- 落盘测试（如 `web::tests` 直写 `config.json` / `models.json`）**必须**测试隔离：`config_dir()` / `pi_dir()` / `models_path()` 在 `#[cfg(test)]` 下重定向到 `temp/pi-switch-test-<pid>`（参考 `src-rust/proxy.rs:115 init_test_state_dir()`，`config.rs:460` 为未隔离反例；曾用 `PI_SWITCH_CONFIG_DIR` 环境覆盖后被 `20f6f86` 误删，即回归）
- 若暂未隔离，CI 侧以 `cargo test --release --lib -- --test-threads=1` 串行化为**过渡**（`2d68f62` 方案，322/322 稳定），并在代码侧记录 `TODO(ci-guard): 恢复 config_dir 测试隔离后去掉 --test-threads=1`
- `verify` 必须跑 `cargo test --lib`（或 `--release --lib` 与发布一致），不跳过；`build` 矩阵 5 目标仅验编译，不代验测试

**完成条件**：
- [ ] 本地 `cargo test --lib -- --test-threads=1` 322/322 且 `cargo test --lib`（并行）亦 322/322 或已记录隔离 TODO
- [ ] `cargo clippy --all-targets` 0 warning
- [ ] `npm run build:webui` 在 `verify` 与 `build` 均执行（本仓 WebUI 缺失会导致 `publish` 产物不一致）

---

### ③ 发布后自纠 —— 发出去的包自己负责

**发布时**：
- `npm publish --access public` 仅在 `if: startsWith(github.ref, 'refs/tags/v')` 且 `needs` 全绿时执行
- 发布前 `actions/download-artifact` 校验 `if-no-files-found: error`，发布后 `npm view <pkg>@<version> version` 回读确认

**自纠**：
- 失败即 **阻断**：`verify` 红 → `publish` 不执行（由 `needs` 保证）；`publish` 自身失败（`409 already exists` / `401`）→ 工作流整体 `failure`，不静默
- 发布后 30s 内 `curl https://registry.npmjs.org/<pkg>/<version>` 校验可用；失败则 `gh issue create --title "chore(release): vX.Y.Z 发布后自检失败" --body "run: ${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}"` 并 `gh release delete vX.Y.Z --yes`（或 `npm unpublish <pkg>@<version>` 在 72h 内）
- `workflow_dispatch` 支持 `inputs.rollback_version` 手动回滚

**完成条件**：
- [ ] `npm view` 回读与 tag 一致
- [ ] 失败路径有 issue/通知（非静默）
- [ ] `git tag` 与 `package.json version` 一致（`scripts/release.sh` 或 `npm version` 保证）

---

## 反模式

- **单依赖 publish**：`needs: build` 是本仓 7 次带病发布的根因
- **仅 tag 触发**：`push.branches` 缺失导致主干修复无 CI
- **真实落盘并行测试**：无 `#[cfg(test)]` 隔离的 `config_dir` 直写是偶发红的根因，`--test-threads=1` 只是止血
- **静默发布**：`publish` 失败不建 issue / 不删 tag，下次 `409` 叠加
- **`-A clippy::all`**：掩盖真实告警

## 引用

- 调研：`.scratch/research/ci-actions-调研.md`（23 次运行全量、`proxy.rs:115` vs `config.rs:460` 对比）
- 修复：`2d68f62 fix(ci): gate publish on verify and serialize Rust tests`
- 关联技能：`diagnose-fix`（通用诊断）、`commit-check`（提交前门禁）、`tdd`（测试隔离后的回归）

## 执行清单（粘贴即用）

```markdown
- [ ] .github/workflows/ci.yml: on.push.branches: [main] 已加
- [ ] publish.needs: [build, verify]
- [ ] verify: cargo test --release --lib -- --test-threads=1（或已隔离则去掉该 flag）
- [ ] config.rs: #[cfg(test)] config_dir/pi_dir → temp（或 TODO 已记录）
- [ ] workflow_dispatch 可手动触发
- [ ] npm view 回读 + 失败建 issue
```
