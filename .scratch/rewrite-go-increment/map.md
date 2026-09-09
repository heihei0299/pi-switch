# Wayfinder Map: Go 重写增量 — 后端前端详细对接

## Destination

产出 **后端-前端详细对接的决策完备 Spec**（含 API 合约、WebUI 状态同步、错误处理、实时统计与网关发布的端到端契约），并落实 `2f113a1` 前对话中的遗留问题（`SupplierCreditsPanel` percent guard、`handleFetchModels` 真实 `GET <baseUrl>/models`、`handleGetCredits` 3 窗口、`new-provider` 残留、`embed.FS` 非法 `all:../../webui/dist`、以及 `multi_supplier` 网关 `3 vs 2` 与 `failover` 热更新），达到可直接 `tdd-implement` 的程度——不改代码，只锁决策。

## Notes

- **域**：`pi-switch` Go 重写（`gin` + `modernc/sqlite` + `embed.FS`）与 React WebUI（`webui/dist` 经 `webui/embed.go`）的契约；术语以 `CONTEXT.md` 为准（供应商/上游/网关/网关发布/余量/统计窗口）
- **每会话必读**：`CONTEXT.md`、`docs/agents/domain.md`、`.scratch/rewrite-go/spec.md`、`internal/server/server.go`、`webui/src/components/*.tsx`、`tmp/handoff-rewrite-go-2026-09-01.md`
- **偏好**：保持 `feat/rewrite-go@2f113a1` 的 Go 单文件 `config.json` / `requests.db` 不破，`go vet` + `go test 66 passed` + `webui 5` 为回归基线；`gin` 保持 `TestMode` 隔离，`embed.FS` 必须 `//go:embed dist` 在 `webui` 包内
- **涉及技能**：`grilling` / `domain-modeling` / `codebase-design` / `research` / `prototype`

## Decisions so far

- [Go 重写基座 09-14 已交付](.scratch/rewrite-go/map.md)：`gin` + 布局、`JSON per-request`、`api-key+AuthProvider`、`embed.FS /api/*`、`translator` 复用、`modernc/sqlite`、`two-stage build`、`bubbletea`，`09-14` 均 `resolved`（`a1c7c26`→`3e9c305`）
- [handoff 2f113a1 增量修复](../../tmp/handoff-rewrite-go-2026-09-01.md)：`SupplierCreditsPanel` `percent` undefined guard、`handleFetchModels` 真实 `GET <baseUrl>/models`（enrich stub）、`handleGetCredits` 3 窗口（`rolling 20% / weekly 45% / monthly 70%`），`new-provider` 残留已清
- [API 合约与校验](issues/01-api-contract.md)：手写 `types.ts` 同步 + `responsesMode` 400 仅校验 + `validate` 仅提示 + `gateway publish` 写前空写后含 2 模型契约
- [WebUI 状态同步](issues/02-webui-state.md)：`SWR` + `preview pending` + `mutate` 双键 + `debounce 300ms` 预览
- [实时统计](issues/03-stats-realtime.md)：轮询 `5s/30s/5min` + 窗口透传保留 + `3-window` 余量 + 全量重算
- [网关与供应商 UI](issues/04-gateway-profiles-ui.md)：`preview pending` 差分+`merge_gateway_extra` 保留、`exposedModels` 主真源/`modelMap` 透传、`spoof` preset 与 `headers/Upstream` 合并、`handleFetchModels` 真实 `GET /models`+分字段 enrich、`+Add` 全选防 `2 vs 3` 残留
- [构建与嵌入一致性](issues/05-build-embed.md)：`dist` 不入库+两阶段强制、`webui/embed.go: //go:embed dist` + `fs.Sub(webUIFS,"dist")` 正解、`/` no-cache vs `/assets/*` immutable、`NoRoute` SPA 200 与 `/api/*` 404 分流、`GET /api/buildInfo` 哈希校验
- [TUI 与 Daemon 后端契约](issues/06-tui-daemon.md)：`list.Model + 3-tab` 的 `tea.Msg` 流与 `FilterValue` 隔离、`~/.pi-switch/*.pid|*.log` + `kill -0/tasklist` + `DialTimeout 500ms` 双探活、`start --daemon` 的 `health retry vs EADDRINUSE 500` 区分、`conhost` 与 `pi-switch` 二进制名一致性
## Not yet specified

<!-- 已清空 — 全部 6 票已 resolved，地图收口，可直接 tdd-implement -->

## Out of scope

- `Home` 集群、`WebRTC`、`Redis Queue`、`Pion` 等 `CLIProxyAPI` 通信基础设施
- `Go SDK` 可嵌入能力（首版仅独立守护进程 + HTTP API）
- 多供应商 `OAuth`（`Codex/Claude/Grok`）与客户端多 `api-keys` 列表认证
- 性能压测与 `MAX_TREE_ENTRIES 500k` 极限调优
