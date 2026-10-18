# pi-switch 架构分析

- 基准：`main @ c3d9897`（2026-09-13）
- 范围：`cmd/`、`internal/`、`webui/`、`docs/`（静态分析，未跑 build/test）
- 导航以 `archive/docs/architecture.md` 为准；边界不变量以 `archive/docs/system-contract.md` 与 `docs/adr/` 为准，本文不重复定义。

## 1. 一句话定位

单 Go 二进制 + 内嵌 WebUI 的双端口网关：`config.json` 存供应商事实，`models.json` 是显式发布的派生视图，`requests.db/SQLite` 存请求事实。

```text
CLI(cmd/pi-switch) ─┐
TUI(internal/tui) ──┼→ internal/server (Gin)
WebUI(React/Vite) ──┘         ├─ :43110 管理面 /api/*
                              └─ :43112 推理面 /v1/*, /anthropic/*
                                       ↓
config(事实) → gateway(派生) → translator(协议转换) → limit/retry → store/stats/catalog
```

## 2. 分层现状（实测行数）

| 层 | 规模 | 评价 |
|---|---|---|
| `internal/server` | 68 文件 / 14524 行（`proxy_handlers.go` 917、`server.go` 735、`profile_handlers.go` 629） | 最胖，需设熔断 |
| `internal/config` | 10 文件 / 1566 行 | 好：唯一读写边界（`ENOENT→Default`，损坏→`error`，`0600+Temp+Sync+Rename`） |
| `internal/protocol` | 2 文件 / 240 行 | 好：`IsKnown/CanProxy/CanGateway/ValidateResponsesMode` 唯一能力源 |
| `internal/gateway` | 6 文件 / 2209 行（`gateway.go` 1175） | 语义对，类型仍是 `map` |
| `internal/translator` | 9 文件 / 2232 行 | 好：`Registry` 集中转换 |
| `internal/profile` | 7 文件 / 896 行 | 好：只迁多入口消费者，不过度抽象 |
| `internal/stats` + `store` | 916 + 562 行 | 好：`TUI→stats→store`，TUI 无 SQL |
| `cmd/pi-switch` | `main.go` 985 行 | CLI 版 god file，需拆文件 |
| `webui/src` | `ProfilesPanel.tsx` 1101 行 / `StatsPanel.tsx` 1106 行，`types.ts` 355 行手工镜像 `config.go` | 巨组件 + 手工镜像，靠 parity test 锁 |

已落实好的（见 maintainability 复审链，已 CLOSED，约 9.3/10）：写门统一 `ValidateResolvedCapability`、Gateway Generated/Draft 显式语义（Generated 用 `FillOverwrite`、Draft 用 `FillMissing`）、错误信封分面（管理 `{error}`／推理 `{error:{message}}`）、无 `app/UseCase/Repository/DI` 层。

## 3. 改进项

### P1 体积回弹（已触发观察项）

1. `server.go` 从拆分后的 522 行涨回 735 行；`proxy_handlers.go` 917 行（鉴权/路由/重试交织）。
   建议：只拆文件不拆层（按 `auth/route/retry` 拆分），加 `>800 行` 告警线，不建新 service。
2. `cmd/pi-switch/main.go` 985 行。建议：延续 `provider-*.go` 模式拆文件，`main` 只留路由。
3. WebUI 巨组件：`ProfilesPanel`／`StatsPanel` 均超 1100 行，测试文件 900–1600 行，已满足 `architecture-review.md §5` 的“churn 上升则拆”条件。
   建议：先拆 `StatsPanel（表格/聚合/明细）`、`ProfilesPanel（UpstreamEditor/ModelCard）`，不动状态模型。

### P2 诚实性残留

4. `POST /gateway/start` 恒返回 `200 {running:true, mode:logical-isolation}`（`gateway_handlers.go:291`，相关 honesty test 已标记）。
   建议：要么接 `daemon` 真状态，要么改 `501 + not_implemented`，不留 `200` 假成功。

### P3 已 defer，立触发器即可

5. `gateway.go` 全量 `map[string]interface{}`：全量类型化已明确不做。新增字段必须走 `Build*Plan` + 补 Draft 显式优先回归。
6. `types.ts` 手工镜像：TS 生成已明确不做。`config.go` 改结构必须同步 `types.ts`，靠 Go parity fixture 卡住。
7. `legacy_log.go` 373 行：`requests.log` 双写是迁移包袱，SQLite 稳定后删，不再加新格式。
8. `docs/maintainability-*` 6 份讲同一事，已是历史。建议归档到 `docs/archive/`，只留 `architecture.md / system-contract.md / adr/` 为活文档。

## 4. 明确不做

`Proxy` 全量重构、`internal/app` 分层、`SQLite migration framework`、`OpenAPI` 生成、WebUI 全面重构 —— `docs/architecture-review.md §5` 与收尾计划已禁。仅当以下条件触发时重估：config 读取经 QPS/延迟数据证明是热点、per-conversation 熔断立项、认证模型变为多用户。
