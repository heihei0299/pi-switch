# T05 旧 WebUI/WebView 与构建链退役——范围审计（2026-10-11）

## 状态与边界

- 本次仅做非破坏性消费者盘点和兼容性决策整理；没有删除、重命名、禁用或改变运行时表面、API、CLI、npm 包或构建入口。
- T02、任务 2（Stats/Sessions/Packages）及 T03 的正式实机 GUI/Wayland/IME/托盘/缩放/受控 Proxy 生命周期验收：**PENDING（本轮未运行，不是 PASS）**。历史 Ubuntu/Xvfb 分项记录不替代正式门禁。
- 用户授权在这些门禁待定时继续 T05 审计；这不等于验收通过、发布就绪或授权移除兼容表面。
- `mygo-t02-native-standard-2026-10-10.md` 已有未提交改动。本次先审阅其 diff；既有历史段落与 Xvfb 结果均保留，只增加当前门禁状态说明。

## 当前消费者与耦合

| 表面 | 仓库证据 | 退役影响 / 安全边界 |
| --- | --- | --- |
| Desktop WebView 回退 | `desktop/main.go` 仅在 `--web` 路径调用 `registerManagementUI`；`desktop/management.go` 注册 `pi-switch-ui://` 并挂载 `server.NewMgmtRouter()`；`desktop/main_test.go`、`desktop/management_test.go` 覆盖回退。`desktop/README.md` 将 `--web` 作为回滚入口。 | 移除会切断已有回退路径；未获兼容决策前保留。 |
| 浏览器 WebUI 与管理 API | CLI `webui start|stop|status` 启停 `server.NewMgmtRouterWithAuth`，默认管理端口 `43110`；`internal/server/server.go` 同一 router 同时注册 `/api/*`、`/assets/*` 与根路径 SPA fallback。非 loopback 绑定有认证保护。 | 浏览器静态 UI 与 HTTP 管理 API 共用 router；不能因退役 React 就假定 `/api` 可删除。API 客户端、脚本及 HTTP 合同的兼容范围未知。 |
| Headless Proxy / inference API | `internal/server` 另有 `NewProxyRouterWithAuth`，提供 `/v1/*`；`pi-switch proxy start|stop|status` 是独立 CLI 生命周期入口。 | 不与 WebUI 静态资产混为一谈；不能因 T05 删除 WebUI 而删除或破坏 Proxy/推理 API。 |
| Go 静态资源 embed | `webui/embed.go` 用 `go:embed dist`；`internal/server` 初始化时读取 index/assets，并提供 `/api/webui/info`、静态资源和 fallback。`cmd/pi-switch`、Desktop WebView registrar 都依赖 `internal/server`。 | 移除 `webui/dist` 会影响 Go 编译/CLI/Desktop 链接和 build identity；需先拆清 API server 与资产服务，再调整相关测试和编译依赖。 |
| npm / pi 分发 | 根 `package.json` 标识 `@heihei0299/pi-switch`，`bin/pi-switch.js` 启动 Linux amd64/arm64 Go binary；`files` 仍含 `webui/dist/`。README/README_ZH 说明 npm 与 `pi install npm:@heihei0299/pi-switch`。CI 有 npm package check，tag 流程配置 npm publish 与 GitHub Release。 | 这是公开安装/升级表面。仓库证据只能证明当前包与发布工作流配置，无法证明使用者是否依赖；不能擅自停发或改变包内容。 |
| TUI | CLI 暴露 `pi-switch tui`；`internal/tui` 使用 Bubble Tea，提供 profile、Gateway 与 Stats 的终端交互，直接依赖 config/gateway/stats，而非 React 页面。 | 与 WebUI 不同的用户可见接口；保留或退役需明确决定。 |
| WebUI / Desktop 构建链 | 根 `npm run build` 构建 WebUI 再构建 Go；Root CI 执行 WebUI typecheck/unit/Playwright 并构建 Go/npm artifacts。Desktop CI 安装 WebUI 与 Bun 依赖，先构建 WebUI embed，再测试 Go；打包前还运行 `desktop` 的 `build:web`。`desktop/mygo.config.ts` 配置 `frontendDist: "dist"`，Native UI 使用 Go MyGo runtime。 | React/Vite runtime 可否删除、MyGo CLI/runtime 与 Desktop `build:web` 哪些仍是 Native 打包必需，需要分开验证；当前不能按名称批量移除。 |
| 文档与测试消费者 | 根 README、中文 README、`WEBUI_GUIDE.md` 描述浏览器入口；CLI help 描述 `webui`/`tui`；`internal/server/build_embed_test.go`、management/API tests、WebUI tests、Desktop fallback tests 分别覆盖这些表面。 | 退役需同步帮助、升级/回滚文档、API 合同测试和 CI；历史测试记录须保留，不能把未完成门禁写成通过。 |

## 需要用户明确的兼容决策

1. **npm / pi 包**：是否继续发布 `@heihei0299/pi-switch` 作为 Linux amd64/arm64 无头 CLI？若保留 CLI 包，是否同时从 npm 包中移除 `webui/dist`？若停发，现有全局 npm 与 `pi install` 用户的迁移/支持期限是什么？
2. **`pi-switch webui`**：`webui start|stop|status` 应删除、弃用，还是改为只提供管理 API？如果继续监听 HTTP，旧浏览器根路径 `/` 应保留、重定向，还是返回明确的退役说明？
3. **HTTP 管理 API**：`/api/*` 是否作为独立兼容合同继续支持外部脚本/客户端？哪些路由有承诺、弃用周期和认证边界？目前 `/api` 与 SPA 同在 `NewMgmtRouterWithAuth` 中。
4. **TUI**：是否继续保留 `pi-switch tui`（profile/Gateway/Stats），以及 CLI/TUI 的功能边界是什么？
5. **Desktop 回退和升级**：`--web` / `pi-switch-ui://` 作为回滚入口保留多久？移除时允许哪些用户可见变化，旧版本如何安装或回滚？
6. **构建链**：确认前五项后，再逐项决定 React/Vite/Playwright、Go embed、MyGo `build:web` 与打包步骤的去留；MyGo Native 运行时/CLI 的保留不能由“WebUI 退役”推断为可删除。

## 决策前可安全推进范围

继续只读引用盘点、测试依赖映射和迁移方案比较。当前停在不改变兼容表面的安全边界：不删除 `webui/`、`webui/dist`、`internal/server` 路由、CLI/TUI、npm 发布配置或 Desktop `--web` 回退；不改 npm/Release/CI 发布行为，不 push、merge 或发布。用户回答上述兼容决策后，再将获准的表面拆成独立任务和提交。
