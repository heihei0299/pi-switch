# pi-switch：macOS 全面退役与 Linux-only GitHub Actions 验证计划

> **文档性质**：实施计划（非执行记录）  
> **仓库**：[`heihei0299/pi-switch`](https://github.com/heihei0299/pi-switch)  
> **工作分支**：`mygo`  
> **审查基准**：`3a6075ad607fc38817a1bb3921209bb2de8f9166`  
> **制定日期**：2026-10-09  
> **目标**：整个项目新版本只支持 Linux，不再构建、分发或维护 macOS/Windows 专用实现。

## 1. 范围和原则

这次仅执行平台退役、必要的引用清理和验证，不进行功能扩展或无关重构。Windows 支持已在前一轮退役；本轮的主要工作是清除**仍然存在的 macOS CLI/npm、构建矩阵和 daemon 代码**，并确保 Linux 功能没有回归。

| 功能 | Linux | macOS | Windows |
| --- | --- | --- | --- |
| CLI / TUI | 保留 | 移除支持 | 已退役 |
| Headless Proxy / WebUI | 保留 | 移除支持 | 已退役 |
| MyGo Desktop | 保留 | 已退役 | 已退役 |
| npm 新版本二进制 | amd64、arm64 | 不再包含 | 不再包含 |
| GitHub Actions 构建 | amd64、arm64 | 不再构建 | 不再构建 |
| 平台专用 daemon 实现 | 保留 Linux 实现 | 删除 Darwin 实现 | 已退役 |

**不做的事**：不修改历史 Git 提交、已发布 npm 版本、已有 GitHub Releases；不修改真实用户配置和 SQLite 数据；不重写已经通过验证的 Linux PID、`flock`、`pidfd`、`Start`、`Stop` 和健康检查逻辑。

**执行规则**：每项独立提交，先做该项定向验证；最终只做一次完整回归。用户已明确授权按本计划测试并发布；不得 force push 或合并 `main`，正式发布只能通过现有 tag workflow，并且所有发布门禁必须先通过。

**与现行计划的关系**：基准提交中的 `docs/plans/mygo/README.md`、`architecture.md`、`verification.md` 和 `wp-07-distribution.md` 当时仍写有 macOS CLI/npm 支持不变；开始 TASK-01 前须明确批准以本计划的 Linux amd64/arm64 范围取代这些约束。用户已于 2026-10-09 明确授权执行本计划，因此该前置条件已满足。实施只退役非 Linux 平台支持，不删除 CLI、TUI、npm 包、浏览器 WebUI 或 MyGo 桌面功能。

## 2. TASK-01：退役 macOS 构建与分发

**优先级**：P1  
**目标提交**：`chore(distribution): retire macos support`

### 修改范围

- `.github/workflows/ci.yml`
- `scripts/build-all.sh`
- `scripts/build-go.sh`
- `bin/pi-switch.js`
- npm 包内容校验逻辑（当前位于 CI 发布流程）；新增的非发布包校验 Job
- `scripts/pi-switch.test.js`：CLI 启动器平台选择与拒绝行为测试（使用 Node 内置测试运行器，不加依赖；避免把测试文件打进 `bin/` npm 包内容）

### 具体要求

1. 从 Root CI 的 CLI 构建矩阵删除 `darwin-amd64`、`darwin-arm64`，只保留 `linux-amd64`、`linux-arm64`。
2. `scripts/build-all.sh` 只生成两个 Linux 二进制。
3. `scripts/build-go.sh` 只接受 `linux/amd64` 与 `linux/arm64`，其他 `GOOS/GOARCH` 组合在创建输出文件前返回非零状态。不要影响 Linux 两目标的正常构建。
4. `bin/pi-switch.js` 只接受 Linux amd64/arm64；其他系统或架构必须在读取 `PI_SWITCH_GO_BIN`、搜索候选可执行文件之前明确报不受支持，避免误选平台不匹配或历史遗留二进制。
5. 修改 npm 包校验：要求两个 Linux 二进制均存在，并拒绝包内任何非这两个受支持目标的 `pi-switch-<goos>-<goarch>` 二进制；旧版本、旧 Release 附件不回溯修改。
6. 让同一 npm 包内容校验在普通非发布 CI（包括 `workflow_dispatch`）中执行，而不只留在仅 tag 触发的 `publish` Job；发布 Job 仍须在 `npm publish` 前校验实际待发布包。不得为验证而创建 tag 或执行发布。
7. 将 `node --test scripts/pi-switch.test.js` 接入 Root CI，确保启动器拒绝行为随普通 CI 持续验证。

### 定向验证

```bash
bash scripts/build-all.sh
GOOS=linux GOARCH=amd64 bash scripts/build-go.sh bin/pi-switch-linux-amd64
GOOS=linux GOARCH=arm64 bash scripts/build-go.sh bin/pi-switch-linux-arm64
```

同时验证 macOS、Windows 和不受支持 Linux 架构请求均被拒绝，且不生成目标二进制：

```bash
for target in 'darwin amd64' 'windows amd64' 'linux 386'; do
  read -r goos goarch <<< "$target"
  output="bin/pi-switch-${goos}-${goarch}-must-not-exist"
  if GOOS="$goos" GOARCH="$goarch" bash scripts/build-go.sh "$output"; then
    echo "FAIL: unsupported target accepted: $goos/$goarch" >&2
    exit 1
  fi
  test ! -e "$output"
done
```

运行 `node --test scripts/pi-switch.test.js`，覆盖 Linux amd64/arm64 选择以及非 Linux/不支持架构在二进制解析前拒绝。执行 `npm pack --dry-run --json`，确认包内含 `bin/pi-switch-linux-amd64` 和 `bin/pi-switch-linux-arm64`，且不含其他平台二进制。普通分支 CI 必须实际运行这项校验，不能用 tag 发布 Job 的结果替代。

### 完成条件

- 只有 Linux 两目标的构建矩阵；
- 新版本 npm 产物仅包含受支持平台二进制；
- macOS/Windows 调用 CLI 有清晰的不支持提示；
- 没有影响 Linux 构建身份、版本信息及 CLI 启动行为。

## 3. TASK-02：移除 Darwin daemon 专用实现

**优先级**：P1  
**目标提交**：`refactor(daemon): remove darwin process support`

### 修改范围

- `internal/daemon/process_identity_darwin.go`：删除
- `internal/daemon/process_kill_darwin.go`：删除
- `internal/daemon/process_identity_other.go`：删除不再需要的 Darwin 兼容桩
- `internal/daemon/daemon.go`：移除 Darwin 特有的调用及平台分支
- `internal/daemon/operation_lock_unix.go`：将构建标签缩窄为 Linux
- 与上述改动直接相关的测试、导入和编译引用

此外检查 `internal/config/lock_unix.go` 的平台构建标签：如其共享 Unix 实现仍写为 `!windows`，在 Linux-only 目标下可收窄为 `linux`；不要因平台退役修改正常的文件锁行为。

### 具体要求

1. 删除 Darwin 专用进程身份和终止实现，确保 Linux 路径不再引用已删除的函数。
2. 保留 Linux `/proc` 进程身份判定、`flock` 锁与 pidfd 强制停止行为。
3. 不引入新的 daemon 状态机、IPC、进程管理抽象或无关业务修改。
4. 删除不再适用的 Darwin 特有测试断言；必须保留真实有效的 Linux 安全断言。
5. 不为了兼容已退役平台而新增空操作桩。

### 定向验证

```bash
go test ./internal/daemon -count=1
go test ./...
go vet ./...
```

重点确认：

- Linux 极值 PID、旧 PID 文件处理、进程身份校验测试通过；
- Linux 停止操作不存在重复 Kill；
- pidfd 强制停止、身份变化拒绝、信号失败、退出确认测试通过；
- 进程操作锁仍能阻止并发修改，退出后正常释放。

### 完成条件

- Darwin 专用 Go 文件和可执行代码引用消失；
- Linux daemon 所有现有测试通过；
- 未引入与平台退役无关的改动。

## 4. TASK-03：统一当前文档为 Linux-only

**优先级**：P2  
**目标提交**：`docs(platform): document linux-only support`

### 修改范围

- `README.md`
- `README_ZH.md`
- `desktop/README.md`
- `docs/plans/mygo/README.md`、`architecture.md`、`acceptance.md`、`verification.md`、`wp-07-distribution.md` 中仍具约束效力的支持范围、构建与发布说明
- `docs/plans/mygo/wp-08-retirement.md` 中仍引用旧 CLI 构建矩阵的当前验收条件；只更新矩阵，不改变 WebUI 退役的延后决定

### 具体要求

1. 平台徽章、安装说明、构建列表统一为 Linux-only。
2. 删除或修正“macOS CLI/npm 支持不变”“CLI 四目标构建”“六目标构建”等失效描述。
3. 将当前构建矩阵明确为 Linux amd64/arm64 两目标。
4. 明确 Windows/macOS 已退役，不再提供新版本支持。
5. 只修订当前生效的约束和验收矩阵。带日期的测试报告、历史 CI 证据及描述当时状态的 handoff/progress 可保留原文，不得把旧构建测试结果改写成当前支持承诺。

### 定向验证

```bash
rg -n 'darwin|macOS|macos|windows-latest|macos-latest|六目标|四目标' \
  .github scripts bin internal README.md README_ZH.md desktop/README.md docs/plans/mygo
```

注意：搜索结果并非必须为零。用于说明“macOS 已退役”的文字、历史记录和不支持平台拒绝测试属于允许项；重点确认**不存在仍声称支持 macOS 的当前规范和执行分支**。

### 完成条件

- README、Desktop 说明和现行发布规范一致；
- 没有把历史版本的支持范围错误改写为 Linux-only；
- 文档改动不改变业务代码与测试逻辑。

## 5. GitHub Actions 最终验收

继续使用现有两套工作流，不新增 CI 系统。

### 5.1 Root CI（`.github/workflows/ci.yml`）

应完成以下检查：

| 检查项 | 验收要求 |
| --- | --- |
| `go test ./...` | PASS |
| `go vet ./...` | PASS |
| WebUI typecheck 和单元测试 | PASS |
| Playwright E2E | PASS |
| CLI 功能 smoke | PASS |
| 秘密扫描 | PASS |
| Linux amd64 构建 | PASS |
| Linux arm64 构建 | PASS |
| 构建身份检查 | PASS |
| 非发布 npm dry-run 内容检查 | 在普通分支/手动运行中通过；包含两个 Linux 二进制，不包含其他平台二进制 |
| macOS / Windows 构建 Job | 不存在 |

**重要**：当前 Root CI 的 `push`/`pull_request` 仅针对 `main`，`mygo` 普通 push 不会自动运行。TASK-01 必须把 npm 包校验放进普通非发布 CI 路径；完成所有提交并经授权推送后，再对 `mygo` 显式触发工作流，并核实运行的 `headSha` 等于最终 HEAD：

```bash
gh workflow run ci.yml --ref mygo
```

本轮包含一次 Linux-only CLI/npm 发布：先完成 TASK-01～03、版本身份校验、Root CI 和 Desktop CI 的最终 HEAD 验收，再创建与 `package.json` 版本一致且 Registry 尚未占用的 tag。只通过现有 tag workflow 发布 npm 并创建 GitHub Release；不在本地手动运行 `npm publish`，不发布 MyGo Desktop 包。未通过任一门禁时不得打 tag。

### 5.2 Desktop CI（`.github/workflows/desktop.yml`）

保持现有 Linux-only 设计，不因 macOS 退役重写 MyGo 桌面：

- Desktop Go test、vet、MyGo vet：PASS；
- Linux amd64/arm64 桌面打包：PASS；
- 桌面内嵌 CLI 的 commit 与 target 一致：PASS；
- Linux amd64 安装、覆盖重装、卸载、用户数据保留 smoke：PASS；
- 构建产物秘密扫描：PASS；
- macOS / Windows 桌面构建 Job：不存在。

Linux ARM64 的原生运行/安装和 Linux Wayland/niri、中文输入法、托盘等交互式 GUI 验收仍属于既有发布 Gate；**CI 构建成功不等于实机验收完成**。不要将这些原有验收缺口扩大成新的 macOS 退役任务。

## 6. 执行顺序与提交规则

1. **TASK-01**：修改构建/分发 → 定向验证 → 单独 commit。
2. **TASK-02**：删除 Darwin daemon 实现 → 运行 Linux daemon 定向及 Go 回归 → 单独 commit。
3. **TASK-03**：更新当前平台声明 → 文档一致性检查 → 单独 commit。
4. **发布身份**：在三个任务提交后，按仓库既有日期版号规则选择未发布版本，更新 `package.json`、`package-lock.json` 和 README 版本徽章，单独提交；先用 npm dry-run 校验包内容。
5. **最终回归**：推送到 `mygo` 后等待 Desktop CI，并在 `mygo` 最终 HEAD 上显式运行 Root CI；核对两套 CI 的 Commit SHA、必需 Job 和日志。不得以本地构建替代远程 CI 证据。
6. **正式发布**：确认 CI 与版本预检全部通过后，创建并推送匹配版本的 tag；等待现有 workflow 完成 npm 发布与 GitHub Release 创建，并核对 Registry 版本及 Release 状态。不得手动绕过 workflow。
7. **结束本轮**：发布成功后关闭退役任务；不得因为发现一般性优化机会继续扩大范围。

每个任务报告：修改文件、验证命令及结果、未验证事项与风险、commit SHA。坚持一个问题一个 commit，**commit 不等于 push**。

禁止强制推送、重置分支、合并 `main` 或修改历史构建产物。Tag、Release 和 npm 发布仅在本轮用户授权及全部门禁通过后，按第 6 节流程手动触发，不得绕过 workflow。

**失败处理**：任一定向验证或 CI 必需项失败，停止后续任务并记录失败命令、日志、影响和恢复方案；修复或通过受控 revert 回退当前任务后重新验证，不得跳过门禁。远程 CI、Linux 实机或用户授权暂不可得时标为 `BLOCK`，不宣称完成、不发布。

## 7. 最终验收清单

- [ ] 当前 Root CI 仅构建 Linux amd64/arm64。
- [ ] 新版本 npm 包仅包含受支持 Linux 二进制。
- [ ] 不支持的平台在 CLI 入口被明确拒绝。
- [ ] 不支持的操作系统/架构无法通过构建脚本生成受支持发布包外的二进制；启动器平台拒绝测试通过。
- [ ] npm 包内容校验在非发布 CI 中运行，发布前也校验实际待发布包。
- [ ] Darwin daemon 专用实现已删除，Linux 构建引用完整。
- [ ] Linux daemon 测试通过，现有安全行为未退化。
- [ ] Root CI 在最终 HEAD 上完成并全部必需 Job 成功。
- [ ] Desktop CI 在最终 HEAD 上完成并全部必需 Job 成功。
- [ ] Linux amd64 安装器 smoke 通过。
- [ ] README、现行架构与分发说明统一为 Linux-only。
- [ ] 未修改历史 Release、用户数据或无关业务逻辑。
- [ ] 三个任务各自有独立提交及可核验的验证记录。
- [ ] 发布版本尚未被 npm 占用，tag 与 `package.json` 版本一致。
- [ ] tag workflow 的 npm 发布和 GitHub Release 均成功，并核实 Registry/Release 状态。

### 结束标准

**本轮结束标准**：新版本构建/分发只产出 Linux amd64/arm64，macOS 专用构建与 daemon 实现退役；Root CI、Desktop CI 对同一最终提交通过；Linux-only CLI/npm 版本经 tag workflow 成功发布。本轮不发布 MyGo Desktop 包；现行 Linux GUI/ARM64 实机门禁仍约束未来桌面版公开发布，不能以 CI 绿灯替代。

---

*本文件定义实施范围与验收方式；实施状态以对应 Git 提交、GitHub Actions 运行和 npm Registry 记录为准。*
