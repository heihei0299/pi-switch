# Windows 支持退役与 Linux 稳定性修复计划

**仓库：** `heihei0299/pi-switch`  
**工作分支：** `mygo`  
**基准：** `b512a6f03b5129c329d4ec85e3d6e25dbfa2fa88`  
**执行方式：** 一个问题一个 commit

## 一、执行目标

本轮分成两个阶段：

**阶段 A：删除 Windows 支持**

- 停止生成 Windows 桌面安装包。
- 停止发布 Windows CLI 二进制。
- 移除 Windows 专用 daemon 实现。
- 清理 Windows 测试任务和直接相关文档。
- 不影响 Linux 功能和现有 macOS CLI 构建。

**阶段 B：修复 Linux 实际缺陷**

- 修复 Linux PID 边界问题。
- 消除 Stop 中重复发送终止信号的风险。
- 整理 daemon 启动失败清理逻辑。
- 恢复 Linux CI，并完成合并前回归。

不进行 MyGo UI 重写、额外功能开发或全面架构重构。

---

## 二、具体任务

### TASK-01：移除 Windows Desktop 支持

**优先级：P1**

修改范围：

- `.github/workflows/desktop.yml`
- `desktop/README.md`
- MyGo 规划中直接涉及平台支持的文档

执行内容：

1. 删除 Windows amd64、arm64 两个打包目标。
2. 删除 `windows-daemon` CI Job。
3. 删除仅服务 Windows 安装器的 NSIS 安装步骤。
4. 移除 Desktop CI 中不再需要的 Windows matrix 参数。
5. 将 MyGo 桌面支持目标明确为 Linux amd64/arm64。
6. 保留 WebUI-first 架构及当前 MyGo 界面实现。

验收标准：

- Desktop CI 不再执行 Windows 打包和 Windows daemon 测试。
- Linux amd64/arm64 构建逻辑不变。
- 文档不再宣称 Windows 桌面受支持。

**Commit：** `chore(desktop): drop windows desktop support`

### TASK-02：移除 Windows CLI/npm 分发支持

**优先级：P1**

修改范围：

- `.github/workflows/ci.yml`
- `scripts/build-all.sh`
- `bin/pi-switch.js`
- `README.md`
- `README_ZH.md`

执行内容：

1. 从主 CI 构建矩阵删除 Windows amd64/arm64。
2. 从 npm 发布前文件校验中删除 Windows `.exe` 要求。
3. `build-all.sh` 不再生成 Windows 二进制。
4. CLI 启动脚本在 `win32` 平台直接返回明确的“不受支持”错误，不能误选其他平台二进制。
5. 更新中英文 README 的平台列表。
6. 保留现有 Linux 和 macOS CLI 构建。

特别注意：

- 不修改已经发布到 npm/GitHub Releases 的历史版本。
- 不删除历史安装包或重新发布旧版本。
- 不改变现有 Linux CLI 命令和配置格式。
- 不因移除 Windows 支持而改变 package name 或命令入口。

验收标准：

- 新版本 npm 包不要求 Windows 二进制。
- 新版本 Linux/macOS CLI 可以正常构建和调用。
- Windows 启动时明确报不支持，而不是报找不到二进制或尝试运行错误平台的程序。

**Commit：** `chore(cli): retire windows binary distribution`

### TASK-03：移除 daemon Windows 专用实现

**优先级：P2**

修改范围：

- `internal/daemon/daemon.go`
- `internal/daemon/operation_lock_windows.go`
- `internal/daemon/process_identity_windows.go`
- `internal/daemon/process_identity_windows_stub.go`
- 相关平台测试及 build tags

执行内容：

1. 删除不再使用的 Windows 原生进程身份实现。
2. 删除 Windows 文件锁适配器。
3. 移除 `daemon.go` 中 Windows 专用的 `runtime.GOOS` 分支。
4. 保留 Linux `flock`，不退回旧式 PID 锁。
5. 保留已有 Linux `/proc` 进程身份机制。
6. 为仍受支持的 macOS CLI 保留必要的 Darwin/Unix 实现，避免误删共享代码。
7. 清理不再使用的 Windows 测试分支和注释。

明确禁止：

- 不将 Windows 实现替换成空操作函数来掩盖遗漏。
- 不删除 Linux 仍依赖的 `golang.org/x/sys`。
- 不重写通用配置、代理路由和数据库层。

验收标准：

- Linux daemon 编译、测试通过。
- 原有系统文件锁测试通过。
- 不再存在 Windows daemon 的必需运行依赖。
- macOS CLI 既有构建能力不退化。

**Commit：** `refactor(daemon): remove windows process management`

### TASK-04：修复 Linux PID 越界误判

**优先级：P1**

位置：`internal/daemon/daemon.go` 的 `isAlive()`

已确认的问题：

最新 Linux CI 的 `TestStartClearsDeadLegacyPIDAndContinues` 失败。使用的 PID 为 `4294967295`，进程存活检测将其错误地判断为存活，导致旧状态阻止启动。

执行内容：

1. 在存活检测前验证 PID 的有效范围。
2. 拒绝零 PID、超出 Linux `pid_t` 正数范围的 PID。
3. 使用可靠的系统调用判断进程是否存在，避免通过外部 `kill` 命令的参数解析处理边界值。
4. 正确区分：
   - 进程不存在；
   - 进程存在；
   - 存在但无权限检查。
5. 保持旧 PID 文件清理时不发送终止信号的安全规则。

测试必须覆盖：

- PID 为 0。
- PID 为 `^uint32(0)`。
- 当前进程 PID。
- 已退出的真实子进程 PID。
- 旧 PID 文件清理后允许继续尝试启动。

定向验证：

```bash
go test ./internal/daemon -run \
'TestStartClearsDeadLegacyPIDAndContinues|TestLegacyPIDStateIsNotManagedOrStopped|TestStatusRejectsReusedPIDIdentity' -count=1
```

**Commit：** `fix(daemon): validate linux pid range before liveness check`

### TASK-05：修复 Linux Stop 的重复终止风险

**优先级：P1**

位置：`internal/daemon/daemon.go` 的 `Stop()`、`forceKillManagedProcess()`

当前问题：

停止流程先调用 `proc.Signal()`，又调用外部 `kill`。强制停止时同样先执行 `proc.Kill()`，再执行 `kill -9 PID`。

这种重复发送信号的方式没有必要，并增加了 PID 复用后的错误终止风险。

执行内容：

1. 正常停止阶段只选择一次终止信号。
2. 删除强制停止阶段额外的外部 `kill -9 PID`。
3. 在执行强制终止前核验进程身份。
4. 优先使用 Linux pidfd 等能绑定具体进程的机制，以减少身份检查到发送信号之间的 PID 复用竞态。
5. 如果系统不支持安全的强制终止方式，明确报错，不能悄悄退回不可靠的重复 PID Kill。
6. 终止后核实进程已退出。
7. 失败时保留可诊断状态，不虚报停止成功。

测试覆盖：

- 合法 daemon 正常停止。
- 强制停止路径。
- 进程身份发生变化。
- 进程已自行退出。
- 终止请求失败。
- 强制终止没有重复发送信号。

定向验证：

```bash
go test ./internal/daemon -run \
'TestStop|TestForceKill' -count=1
```

**Commit：** `fix(daemon): eliminate duplicate linux process termination`

### TASK-06：收敛 Linux 启动、端口检测与测试合同

**优先级：P2**

当前 Windows 已退出支持范围，因此不再需要 Windows 专用的 `WSAEADDRINUSE` 兼容修复。

但 Linux 仍需要保留稳定的 `ErrPortInUse` 合同和健康检查安全逻辑。

执行内容：

1. 保留启动前端口可用性检查。
2. 保留启动后监听者身份核验。
3. 明确区分端口被占用、启动子进程退出、健康检查超时三类错误。
4. 保留 `errors.Is(err, ErrPortInUse)` 合同。
5. 不再用历史日志中的无关错误替代当前启动失败原因。
6. 确认启动失败后等待新子进程退出，并清理本次写入的 PID 状态。
7. 修正受上述行为变化影响的测试，但不得删除有效安全断言。

定向验证：

```bash
go test ./internal/daemon -run \
'TestStart|TestWaitForHealth|TestTuiDaemon_S5_Start' -count=1
```

**Commit：** `fix(daemon): stabilize linux startup failure handling`

---

## 三、最终 CI 与验收

### Linux daemon

在全部独立修复完成后：

```bash
go test ./internal/daemon -count=1
go test ./...
go vet ./...
```

所有测试必须通过，不允许通过跳过失败测试恢复绿色。

### MyGo Desktop

先按项目现有流程生成 WebUI 嵌入资源并安装桌面依赖，然后：

```bash
go -C desktop test ./...
go -C desktop vet ./...
```

GitHub Actions 需要完成：

| 验证 | 要求 |
|---|---|
| Linux daemon | PASS |
| Desktop module | PASS |
| Linux amd64 构建 | PASS |
| Linux arm64 构建 | PASS |
| Linux amd64 installer smoke | PASS |
| Root CI Go/WebUI | PASS |
| macOS CLI 构建 | 保持已有能力 |
| Windows 构建、测试、打包 | 不再要求 |

Desktop CI 继续在 `mygo` push 和面向 `main` 的 PR 上运行。

### 回归重点

集中验证：

- 旧 PID 文件不会导致误杀。
- PID 复用不会造成错误的进程管理。
- daemon lock 能可靠释放。
- 代理启动失败不会遗留新子进程。
- Start/Stop/Status 结果与实际运行状态一致。
- 关闭桌面程序不会自动停止独立 Proxy。
- 用户原有配置和 SQLite 数据保持不变。

---

## 四、提交与交付约束

执行顺序：

`TASK-01 → TASK-02 → TASK-03 → TASK-04 → TASK-05 → TASK-06 → 最终回归`

每项独立提交，完成针对性验证后才可 commit。

如果修复过程中发现新的独立 bug，应另立任务，不合入当前 commit。

每项任务报告修改文件、验证命令及结果、未验证项目、风险和 commit SHA。

禁止自动：

- 删除用户未提交、暂存或未跟踪的文件。
- 重置分支或强制推送。
- 合并 `main`。
- 修改 GitHub Releases 历史资产。
- 发布 npm 新版本。

---

## 五、本轮明确不处理的内容

- Windows 兼容性修复及 Windows daemon 测试。
- macOS 桌面相关问题。
- MyGo 原生界面功能扩展。
- WebUI 退役。
- Windows graceful shutdown 或跨平台 IPC 架构。
- 无关数据库、Gateway、供应商管理重构。

**完成标准：** Windows 不再属于项目新版本的支持和分发范围；Linux daemon 的已确认问题解决；Linux 与保留的 CLI 构建测试通过；完整 CI 恢复；每项任务都有独立验证与提交记录。
