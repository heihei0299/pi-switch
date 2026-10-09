# T04 — MyGo Native 成为默认且唯一正式 GUI

- **源需求**：SPEC §2.1(1)、§6 WP-05
- **优先级**：P1
- **前置**：T02 + T03 的所有强制验收已通过
- **交付**：Desktop 直接启动完整 Native GUI；本 Ticket 一个交付 Commit

## 目标与范围

改变正式桌面入口，从“WebView 载入 React 的主窗口 + 可选 Native 原型窗口”切换为“默认打开统一 MyGo Native 应用”。**本 Ticket 切换入口，但不在同一 Commit 删除旧 UI 依赖**（由 T05 单独负责），以保留可审查的回退点。

## 实施步骤

1. 调整 `desktop/main.go`、`desktop/shell.go` 中的正式窗口入口与导航，让 T02/T03 页面处于统一导航结构；清理实验性多窗口之间的重复入口，但不删除尚供 T05 审计的旧前端文件。
2. 以 MyGo Native `Content/ui.View` 为正式 GUI；默认启动、菜单/托盘打开行为均进入 Native，任何正式管理操作不再通过 WebView 或 URL 加载 React。
3. 收敛打开/隐藏/退出、单实例及 Proxy 运行状态：关闭窗口、退出 GUI、用户显式停止 Proxy 是不同操作；不要把退出与停止代理绑在一起。
4. 对完整用户路径做正式验收并登记可用的旧构建/回滚 Commit：Providers/Profile → Gateway 预览/显式发布 → Proxy → Stats/Sessions → Packages/Settings。
5. 为下一 Ticket 输出“旧 UI 资源与构建依赖删除清单”，记录哪些能力依赖旧管理 HTTP 入口、npm 或 `pi-switch webui`。

## 验收（全部必须）

- [ ] 所有正常入口默认进入完整 MyGo Native GUI，主窗口不加载 WebView、HTML、React。
- [ ] 不存在只能通过旧 WebUI 才能操作的正式桌面功能。
- [ ] 关闭所有窗口/退出 GUI 不会误停独立或非托管 Proxy；菜单/托盘能正确显示管理状态。
- [ ] 完整主流程在隔离配置/模型/统计路径中执行成功；未触发自动 Gateway 发布或隐藏写入。
- [ ] T02/T03 的页面级验收结果继续成立；有明确可审查的回退 Commit。
- [ ] T05 删除清单包含 `desktop/index.html`、`desktop/src`、`desktop/management.go`、`webui/`、静态资源嵌入、构建脚本、CI、npm 与 CLI 相关入口；这里只清单化，不提前删除。

## 验证与证据

- `go -C desktop test ./...`；桌面构建成功；在真实 Linux GUI 上从冷启动逐项操作主流程并测试退出/再次打开。
- T04 前先保存基线构建可恢复性证据；回滚采用独立 revert/修复 Commit，而非 reset/强推。

## 不在范围内

删除 WebUI/React/Vite/WebView/HTTP 管理接口、npm/TUI 兼容调整（T05）；完整跨 Linux 发行版发布门禁（T06）。

