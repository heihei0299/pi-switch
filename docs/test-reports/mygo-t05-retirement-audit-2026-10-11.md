# T05 旧 WebUI/WebView 与构建链退役——范围审计（2026-10-11）

## 状态与边界

- 本文先记录 T05 初始的非破坏性范围审计；后续实施与验证见“实施进度更新”。
- T02、任务 2（Stats/Sessions/Packages）及 T03 的正式 Arch+niri/Wayland/IME/托盘/硬件缩放/受控 Proxy 生命周期验收：**PENDING（未通过，不是 PASS）**。本轮仅补充 Ubuntu/Xvfb smoke；历史 Xvfb 分项记录不替代正式门禁。
- 用户授权在这些门禁待定时继续 T05 审计；这不等于验收通过、发布就绪或授权移除兼容表面。
- `mygo-t02-native-standard-2026-10-10.md` 已有未提交改动。本次先审阅其 diff；既有历史段落与 Xvfb 结果均保留，只增加当前门禁状态说明。

## 实施进度更新（2026-10-11）

- 已将 CLI `webui start|stop|status` 与 Desktop legacy UI 分路：CLI start 使用 API-only router，根路径返回 HTTP 410 退役说明；Desktop `NewMgmtRouter()` 仍提供嵌入式 React UI 和 SPA fallback。
- API-only 与 legacy router 的 `/api/*`、health 路由清单及认证回归一致；Proxy `/v1/*` 和 `pi-switch tui` 未改。
- npm 继续分发 Linux CLI。API/UI 路由分离后，npm `files` 不再单独包含 `webui/dist/`；Go `embed.FS` 与 Desktop `--web` fallback 仍保留，因此根/ Desktop 构建目前仍需生成 `webui/dist`。
- Desktop Native 的独立占位 Web frontend 构建链已移除：MyGo 配置改为 `desktop/mygo.json`（不声明 `frontendDist`），删除 `desktop/package.json`/`bun.lock`、Vite/TypeScript/MyGo TS runtime 依赖及空白 HTML/Vite 源；Desktop CI 改由已固定版本的 Go module CLI 执行 MyGo，不再安装 Bun。WebUI 仍单独构建，以满足 legacy `--web` router 所在 Go 包的 embed。
- `scripts/check-npm-package.mjs` 现在拒绝把任何 `webui/` 资源装入 npm 包。隔离 fixture 正向检查通过（6 个包文件），把 `webui/dist/` 重新加入清单的负向检查按预期失败。
- 包清单提交 `47ffa91` 的 Root CI `38140186739` 与 Desktop CI `38140179179` 均成功；Root CI 覆盖 Linux amd64/arm64 构建及 Linux-only npm package check。
- 隔离 npm fixture 检查通过（应为 6 个文件；重新把 `webui/dist/` 加回清单时负向检查失败）。本地工作树的 `npm pack` 因 ignored `bin/` 中残留的 Darwin/Windows 旧构建而列出 11 个文件；本地 package verifier 据此拒绝这 4 个非 Linux 二进制。没有删除或覆盖这些 ignored 文件；干净 CI 的 Linux-only package check 已成功。
- 基于当前 HEAD `1a9cd9c` 的 Ubuntu 24.04/Xvfb/Openbox 补充 GUI smoke：Native 窗口可见，导航访问 Profiles、Providers、Proxy、Gateway、JSON Editor、Stats & Sessions、Packages、Settings、Backups、Doctor 未退出；Profiles 搜索可输入并过滤；Doctor 报告配置有效。隔离证据位于 `/home/gui-test/pi-switch-gui/sandbox/runs/plan-1a9cd9c-20261011-3UNgyT/`，页面截图在 `screenshots/page-*.png`。它不是 Arch+niri/Wayland 验收。
- Xvfb 主机日志显示托盘因缺少/不支持 `libayatana-appindicator3` 不可用；IME、Wayland 与硬件缩放也未验证。正式 T02/任务 2/T03 GUI 门禁继续为 **PENDING（未运行/未通过，不是 PASS）**。
- 同一隔离环境中的 CLI smoke：`GET /` 为 410、`GET /api/config` 为 200、`/assets/*` 与未知 API 路径为 404；测试服务和本轮 GUI 均已关闭，只保留隔离证据目录。此前已有无法核验身份的服务及 GUI 进程未触碰。

## 当前消费者与耦合

| 表面 | 仓库证据 | 退役影响 / 安全边界 |
| --- | --- | --- |
| Desktop WebView 回退 | `desktop/main.go` 仅在 `--web` 路径调用 `registerManagementUI`；`desktop/management.go` 注册 `pi-switch-ui://` 并挂载 `server.NewMgmtRouter()`；`desktop/main_test.go`、`desktop/management_test.go` 覆盖回退。`desktop/README.md` 将 `--web` 作为回滚入口。 | 按用户决策保留，直到正式 GUI 门禁通过且弃用说明发布。 |
| 浏览器 WebUI 与管理 API | CLI `webui start|stop|status` 保留命令名与默认端口 `43110`，但 start 使用 `NewMgmtAPIRouterWithAuth`；根路径返回 410，静态资源不注册。`NewMgmtRouter` 仍由 Desktop `--web` 注册器使用。 | CLI 的静态 UI 入口已退役；所有现有 `/api/*` handler 与认证中间件共享实现并由路由对比/认证测试保护。未知路由不再落入 SPA fallback。 |
| Headless Proxy / inference API | `internal/server` 另有 `NewProxyRouterWithAuth`，提供 `/v1/*`；`pi-switch proxy start|stop|status` 是独立 CLI 生命周期入口。 | 不与 WebUI 静态资产混为一谈；不能因 T05 删除 WebUI 而删除或破坏 Proxy/推理 API。 |
| Go 静态资源 embed | `webui/embed.go` 仍使用 `go:embed dist`；`internal/server` 保留 `/api/webui/info` 与 legacy Desktop 静态资源/fallback。API-only CLI 仍链接同一 `internal/server` 包。 | 本轮只从 npm 文件清单移除单独的 `webui/dist/`，没有移除 embed；Go 二进制及 Desktop fallback 仍含静态资源，后续需拆分 build graph 才能退役 embed。 |
| npm / pi 分发 | 根 `package.json` 保留 `@heihei0299/pi-switch`、`bin/pi-switch.js`、Linux amd64/arm64 二进制与 README；`files` 不再包含单独的 `webui/dist/`。CI package check 限制 Linux 二进制并拒绝 `webui/`。 | npm/`pi install` 无头 CLI 保留；不停止发布、不改 CI/Release 工作流。本次仅改变包内文件清单，不代表发布已发生。 |
| TUI | CLI 暴露 `pi-switch tui`；`internal/tui` 使用 Bubble Tea，提供 profile、Gateway 与 Stats 的终端交互，直接依赖 config/gateway/stats，而非 React 页面。 | 按用户决策保持不变。 |
| WebUI / Desktop 构建链 | 根 `npm run build` 仍构建 WebUI 再构建 Go；Root CI 保留 WebUI typecheck/unit/Playwright。Desktop Native 通过 `desktop/mygo.json` 使用 Go UI，无 `frontendDist`；MyGo CLI 由 `desktop/go.mod` 固定，Desktop CI 不再安装 Bun/npm desktop package。Desktop CI 仍先构建 `webui/dist`，供 `--web` fallback 所在 Go 包 embed。 | 已移除仅用于占位页面的 Desktop Vite/TypeScript/npm 构建链；React、Go embed 与 custom-protocol fallback 暂时保留。 |
| 文档与测试消费者 | 根 README、中文 README、`WEBUI_GUIDE.md` 描述浏览器入口；CLI help 描述 `webui`/`tui`；`internal/server/build_embed_test.go`、management/API tests、WebUI tests、Desktop fallback tests 分别覆盖这些表面。 | 退役需同步帮助、升级/回滚文档、API 合同测试和 CI；历史测试记录须保留，不能把未完成门禁写成通过。 |

## 已确认的兼容决策

1. **npm / pi 包**：继续保留 Linux amd64/arm64 无头 CLI；API/UI 分离完成后从 npm 包移除 standalone `webui/dist/`，不停止 npm/`pi install` 分发。
2. **`pi-switch webui`**：保留 `start|stop|status` 命令作为管理 API 生命周期；根路径显示退役说明，所有现有 `/api/*` 路由和认证行为不变。
3. **TUI 与 Proxy**：`pi-switch tui` 与 Proxy `/v1/*` 保持不变。
4. **Desktop fallback**：保留 `--web` / `pi-switch-ui://`，直到正式 GUI 门禁通过且弃用说明发布；当前门禁 PENDING，因此本轮未删除。
5. **构建链**：在 Desktop fallback 仍依赖 embed 的期间，保留 `webui/dist` 构建与 Go embed；不把“npm 不单独携带静态文件”误解为“构建时不再生成静态资源”。

## 剩余工作与边界

继续按计划验证 Native 正式 GUI 门禁并盘点尚存 build graph。正式门禁未通过前，不删除 Desktop `--web` / custom protocol；未单独授权发布前不发布 npm/GitHub Release。npm 文件清单变更已提交并推送，CI 验证通过；这不代表发布已发生。


## Desktop Native 构建链实施与本地验证（2026-10-11）

- Native window 走 Go `ui.View`，`--web` 走独立的 `pi-switch-ui://` router；两者都不消费 Desktop Vite 输出。根据 MyGo v0.3.3 本地源代码，`frontendDist` 可选，配置不提供它时构建不生成 MyGo `SetFrontend` embed。
- Desktop CI 已改为直接 `go run -mod=readonly github.com/egoist/mygo/cmd/mygo ...`，复用 `desktop/go.mod` 中的版本，不再通过 Bun/npm 安装 MyGo CLI 或构建占位的 desktop HTML。WebUI 构建仍保留，因 Desktop 主 Go 包继续导入 `internal/server`/`webui.FS` 以支持 `--web` 回退。
- 本地验证：`GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off CGO_ENABLED=0 go -C desktop test -mod=readonly ./...`、对应 `go vet`、`go -C desktop run -mod=readonly github.com/egoist/mygo/cmd/mygo vet .` 均通过。MyGo Linux/amd64 package build 输出到 `/tmp/pi-switch-t05-desktop.HMGXmK/build` 成功；构建前后 `webui/dist/index.html` 哈希一致，二进制不含旧占位页面文案，且未生成 `desktop/dist`。既有 `desktop/native_complex_test.go` 用户改动只参与测试，没有纳入本次改动。
- 提交 `6096da3d5f332607021115ac163273a5d7dc8d64` 已推送。Desktop CI run `38141867619` 在该精确 SHA 成功：Go module tests/vet、MyGo vet、secret scan、Linux amd64/arm64 package build 均通过；amd64 installer smoke 通过，arm64 installer smoke 按 workflow 条件跳过。Actions 同时给出 Node.js 20 action deprecation 与 `ubuntu-latest` 将于 2026-10-19 迁移到 Ubuntu 26 的告警；本次工作流本身成功。
- T02/任务 2/T03 正式 GUI gate 仍为 **PENDING（不是 PASS）**；Desktop CI 与 Xvfb smoke 均不替代真实 Wayland/IME/托盘/硬件缩放/受控 Proxy 门禁。
