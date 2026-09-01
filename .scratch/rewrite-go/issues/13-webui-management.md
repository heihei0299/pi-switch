# 13: WebUI 管理面（profile CRUD + 网关预编辑 + 统计窗口）

**What to build:** 在 12 的代理能力上补齐浏览器管理面：保留现有 React WebUI（`vite` 构建产物经 Go `embed.FS` 嵌入 `webui/dist`），管理 API 规范化为 `/api/*`（与现有前端仅改 `baseURL`）：`profiles` 增删改查（含 `preset`/`models`/`modelMap`/`exposedModels`/`headers`/`userAgent` disguise）、`validate_config` 校验反馈、`test_provider`/`fetch_models`、`gateway` 预编辑与 `publish`、统计页 `StatsWindow` 四档（当天/24小时/7天/自定义 `[起日0点,止日24点)`）作用于全部聚合、按对话（`Conversation`）与按请求（`Request Details`）表格（分页）、`formatCost`（`$0.00/$0.0042/$12.34/$1.2K/-`）与 `Cache Hit Rate`、导出 `JSON/CSV` 含消费列、`webui_password` 的 basic auth（非 loopback 时 `admin` + 密码）。`webui/dist` 变更后 `npm run build:webui` 嵌入，无需 Rust 构建。

**Blocked by:** 12 协议转换与限流流式

**Status:** resolved

- [x] `GET /` 返回 React WebUI（`embed.FS`），`GET /api/profiles`/`PUT /api/profiles/:name`/`DELETE /api/profiles/:name` 完整 CRUD 且 `exposedModels` 与 `modelMap` 同步
- [x] `POST /api/gateway/publish` 显式写 `models.json:providers[pi-switch]`，预编辑面板与发布后 `models.json` 一致；`POST /api/validate` 返回 `level/path/message` 列表
- [x] 统计窗口四档过滤正确：`GET /api/stats?window=today|24h|7d|custom&from&to` 的 `totalCost/totalTokens/byConversation/byModel` 均受窗口约束
- [x] 导出：`GET /api/export?format=json|csv` 含 `cost/cached/reasoning` 列，旧行 `NULL` 按空处理；`NODE_ENV=test npx vitest run` 通过（`production` 下 `act` 报错已规避）

## Answer

已实现并验证（`internal/server/webui_management_test.go` 5 用例 + `webui` 199 用例，`go test ./...` 全部通过）：

- `GET /` 经 `webui/embed.go:FS`（`dist`）`embed.FS` 嵌入，`/` 返回 `dist/index.html`（含 `<div id="root">`），`/assets/*` 静态，SPA fallback 保持；`webui/dist` 变更后 `npm run build:webui` 触发 Go 嵌入，无需 Rust。
- `GET /api/profiles` / `POST /api/profiles` / `PUT /api/profiles/:name` / `DELETE /api/profiles/:name` 完整 CRUD，`exposedModels` 与 `modelMap`/`userAgent`/`headers`/`preset` 同步（`internal/config:ProviderProfile` 新增 `ModelMap/UserAgent/Preset/Proxy`），`responsesMode` 不兼容在保存时 400。
- `POST /api/gateway/publish` 显式写 `models.json:providers[pi-switch]`（`gateway.Publish`），`GET /api/models/gateway/preview` dry-run 不写盘，`PUT /api/models/gateway` 校验 `api/baseUrl`。
- `POST /api/validate` / `GET /api/config/validate` 返回 `level/path/message`（`allowedAPIs`、`baseUrl`、`models`、`responsesMode`、`failover` 校验）。
- 统计窗口四档：`parseWindowQuery` 归一化 `window=today|24h|7d|custom`（`24h→last24h`、`7d→last7d`）与 `range` 双别名，`from/to` 为 epoch 毫秒，`tsEpochMs`/`inWindow` 过滤，`totalCost/totalTokens/byProvider/byModel/byConversation` 均受窗口约束，`costUnknown` 统计，`cacheRate` 计算。
- 导出：`GET /api/export?format=json|csv` 与 `GET /api/logs/export` 双别名，JSON 含 `cost/costTotal/cachedTokens/reasoningTokens`，CSV 表头含 `cost/costTotal/cachedTokens/reasoningTokens`，旧行 `NULL` 按空。
- `webui_password` 的 Basic Auth：`isLoopback` 判定 `127.0.0.1/localhost/::1`，非 loopback 时 `admin:` + `~/.pi-switch/webui_password`（或 `PI_SWITCH_WEBUI_PASSWORD`），`Authorization: Basic` 校验。

提交：`feat(rewrite-go): 13 webui management`。下一票：`14 TUI 与发布收敛` 已 unblocked。
