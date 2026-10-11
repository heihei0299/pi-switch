# T03 Native 默认入口与统一导航 — 实施记录（2026-10-11）

## 状态

- **代码实施：完成于当前工作树；T03 整体验收：未通过 / BLOCK。**
- 此前曾要求跳过实机验收；按最新指示，T03 正式 GUI/Wayland/IME/托盘/缩放/受控 Proxy 生命周期验收为 **PENDING**（本轮未运行，不是 PASS）。当前机器未运行 GUI，也未连接测试主机。
- 阶段 0 的 Root CI 与 Desktop CI 已在已提交 HEAD 上通过（详见下表），但 T02、任务 1/2 与 T03 的正式 GUI 门禁均为 **PENDING**（本轮未运行，不是 PASS）。用户授权门禁待定时继续 T05 范围审计，不代表通过或豁免，也不表示 React/WebView 可以退役。
- 任务 2（Stats/Sessions/Packages）与 T03 默认入口/统一导航已按顺序分别提交；原有 unrelated staged/unstaged/untracked 改动未混入。

## 本轮改动

1. 默认启动使用 MyGo Native 主窗口，不注册旧管理协议/路由；旧 React/WebView 通过显式 `--web` 作为回退入口，`--native` 仍可用。
2. 将 Overview、Profiles、Providers、Proxy、Gateway、JSON Editor、Stats & Sessions、Packages、Settings、Backups、Doctor 放入同一个原生窗口导航；Gateway/Stats/Packages 的异步任务更新主窗口，不再创建复杂功能第二窗口。
3. File、tray、第二实例唤醒和激活事件统一显示当前主窗口。窗口隐藏/退出仍不隐式停止 Proxy。
4. 更新 `desktop/README.md` 说明新默认入口、`--web` 回退方式和仍未完成的验收门禁。
5. Native UI 没有导入 `internal/server`；该包只留在 legacy `desktop/management.go` 的 WebView registrar 中。

## 验证

| 检查 | 结果 |
| --- | --- |
| `go -C desktop test ./...` | PASS；包含 Native 主导航路由 UI tester 覆盖、默认 Native/显式 Web fallback、菜单与窗口策略测试 |
| `go -C desktop test -run '^TestNativeStandardNavigationAndEmptyStates$' -count=1` | PASS；在窗口最小宽度 560 DIPs 下逐项访问全部原生导航页面 |
| `go -C desktop vet ./...` | PASS |
| `go -C desktop build -o /tmp/pi-switch-t03-native` | PASS；Linux x86-64 ELF。SHA-256：`80bb992fc5916dd2f99eea099b6a696ad1ef14b1487cfe67bdf3bfe99b86b30f` |
| `GOPROXY=off GOSUMDB=off go test -count=1 ./...`、`GOPROXY=off GOSUMDB=off go vet ./...` | PASS；当前 dirty worktree 的 root Go packages，不安装依赖 |
| `bash scripts/check-secrets.sh` | PASS；检查 Git 跟踪文件（脚本不扫描未跟踪文件） |
| `git diff --check` | PASS |
| Root CI run `38121054078` | PASS；`workflow_dispatch`，HEAD `ae3b9ef934a6fc6cf8909c254f66b2fe7c13415d`，Root CI、Linux amd64/arm64 builds、npm package check 全部成功 |
| Desktop CI run `38120018023` | PASS；HEAD 同为 `ae3b9ef934a6fc6cf8909c254f66b2fe7c13415d`，Desktop module、Linux amd64/arm64 package builds 成功；amd64 installer smoke PASS，arm64 smoke 按 workflow 条件 SKIP |
| 实机 GUI / Wayland / IME / tray / 缩放 / 受控 Proxy 完整流程 | **PENDING（本轮未运行，不是 PASS）** |

以上远程 Actions 针对已提交 HEAD，不包含本报告的 dirty worktree 修改；T03 当前改动由本地 Desktop 测试/vet/build 验证，尚无对应远程 CI run。

首次运行 desktop 测试时，旧 UI tester 仍检查内部验证视图的旧标签 `Gateway Diff`；恢复该测试视图标签后再次运行，测试通过。该内部视图不再作为应用窗口，正式入口通过主导航显示 Gateway。

最后一次导航清理时，测试/vet/build 发现重复 switch case；已修正，并对最终源码重跑上述三项，均通过。

## 独立提交验证（2026-10-11）

Task 2 先提交 `b3f5244`，随后提交 T03；T03 代码在以 Task 2 commit 为基线的隔离源码树中复验：`go -C desktop test ./...`、`go -C desktop vet ./...`、`go -C desktop build` 与 `TestNativeStandardNavigationAndEmptyStates` 均 PASS，提交 diff check PASS。真实 GUI 验收仍为 **PENDING**（未运行，不是 PASS），没有进行 GUI 操作。

## T05 退役前仍存在的旧 UI 消费者

- `webui/` React 源码及 `webui/dist` 构建产物。
- `desktop/management.go` 的 `pi-switch-ui` custom protocol 和 `server.NewMgmtRouter()`；`desktop/management_test.go` 覆盖旧入口。
- `desktop/mygo.config.ts`、`desktop/package.json`、`desktop/vite.config.ts`、`desktop/bun.lock` 仍构建/嵌入 WebUI；根 `package.json`、`scripts/build-all.sh` 仍打包 `webui/dist`。
- `.github/workflows/ci.yml` 与 `.github/workflows/desktop.yml` 仍执行 WebUI build/typecheck/test 或依赖其产物。
- `internal/server` 仍有管理 API、旧 WebUI 路由与相关调用方/测试；T05 前必须先明确浏览器管理入口、npm/CLI 分发和服务端 API 的兼容处置，不能仅因桌面默认改为 Native 就删除。

## 未验证与风险

`--web` 的参数分流和 URL 构造由单元测试覆盖，但没有实机启动旧 WebView。当前 Native 默认入口的真实 GUI/Wayland/IME/托盘/缩放/受控 Proxy 生命周期及 T02、任务 1/2、T03 正式验收均为 PENDING（未运行，不是 PASS）；因此 T03 的实施不构成正式桌面发布资格，也不授权删除旧 UI。
