# 13: WebUI 管理面（profile CRUD + 网关预编辑 + 统计窗口）

**What to build:** 在 12 的代理能力上补齐浏览器管理面：保留现有 React WebUI（`vite` 构建产物经 Go `embed.FS` 嵌入 `webui/dist`），管理 API 规范化为 `/api/*`（与现有前端仅改 `baseURL`）：`profiles` 增删改查（含 `preset`/`models`/`modelMap`/`exposedModels`/`headers`/`userAgent` disguise）、`validate_config` 校验反馈、`test_provider`/`fetch_models`、`gateway` 预编辑与 `publish`、统计页 `StatsWindow` 四档（当天/24小时/7天/自定义 `[起日0点,止日24点)`）作用于全部聚合、按对话（`Conversation`）与按请求（`Request Details`）表格（分页）、`formatCost`（`$0.00/$0.0042/$12.34/$1.2K/-`）与 `Cache Hit Rate`、导出 `JSON/CSV` 含消费列、`webui_password` 的 basic auth（非 loopback 时 `admin` + 密码）。`webui/dist` 变更后 `npm run build:webui` 嵌入，无需 Rust 构建。

**Blocked by:** 12 协议转换与限流流式

**Status:** ready-for-agent

- [ ] `GET /` 返回 React WebUI（`embed.FS`），`GET /api/profiles`/`PUT /api/profiles/:name`/`DELETE /api/profiles/:name` 完整 CRUD 且 `exposedModels` 与 `modelMap` 同步
- [ ] `POST /api/gateway/publish` 显式写 `models.json:providers[pi-switch]`，预编辑面板与发布后 `models.json` 一致；`POST /api/validate` 返回 `level/path/message` 列表
- [ ] 统计窗口四档过滤正确：`GET /api/stats?window=today|24h|7d|custom&from&to` 的 `totalCost/totalTokens/byConversation/byModel` 均受窗口约束
- [ ] 导出：`GET /api/export?format=json|csv` 含 `cost/cached/reasoning` 列，旧行 `NULL` 按空处理；`NODE_ENV=test npx vitest run` 通过（`production` 下 `act` 报错已规避）
