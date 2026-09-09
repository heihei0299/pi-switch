# 10: WebUI 同步与实时统计

**What to build:** 让 WebUI 的状态同步与实时统计可演示：`SWR ["profiles"]/["gateway"]/["stats", window]` 在 `PUT` 后 `mutate`，`StatsPanel` 的 `Off/5s/30s/5min` 轮询 `GET /api/stats` 失败保旧，`SupplierCreditsPanel` 的 `rolling/weekly/monthly` 3 窗口 `percent` 永不 `undefined`。

**Blocked by:** 09 网关与供应商端到端（合并）

**Status:** resolved

- [x] `SWR` 三键 `["profiles"] / ["gateway"] / ["stats", window]`（`window=today|24h|7d|custom 的 from/to`），`PUT /api/profiles|gateway/publish|/proxy/failover` 后 `mutate ["profiles"]` 与 `["gateway"]` 双键，浏览器 `ProfilesPanel` 的 `exposed` 徽标即时一致
- [x] 乐观更新取等待确认（`PUT` 后等待 `GET /api/profiles` 成功再更新本地列表），避免与 `gateway publish` 的 `exposedModels` 竞争
- [x] `userAgent` 的 `debounce 300ms` 后 `modelPreview(draft)` 合成 `preview headers` 的 `X-Custom` 合并，`Upstream` 聚合时可见，不自动写盘
- [x] `StatsPanel auto-refresh Off(默认)/5s/30s/5min` 四档（`setInterval`），`modernc/sqlite` 无 `LISTEN` 不引入 `WebSocket`，`window` 参数在轮询间透传，`auto-refresh` 失败保留旧数据与旧窗口
- [x] `GET /api/stats?range=&from=&to=&page=&limit=` 与 `GET /api/stats/conversations` 的 `parseWindowQuery` 四档校验，`byConversation/byModel` 每次轮询全量重算（`okRequests vs total`，`recentRequests` 分页），`totalCost` 仅 `success=1 && prompt+completion可解析` 累加，`costUnknown` 计 `success=1 && cost==nil`
- [x] `handleGetCredits → SupplierCreditsPanel` 3 窗口 `rolling 20% / weekly 45% / monthly 70%`（`percent=已用/总量`，`undefined→0 guard`），`cacheRate` 为 `cached/input` 的 `"-" | "0.0%" | "x.x%"`，`formatCost: 0→$0.00, 0.0042→$0.0042, 12.34→$12.34, 1234→$1.2K, null→"-"`
- [x] 前端 `NODE_ENV=test npx vitest run`（`production` 下 `act` 报错）覆盖 `GatewayPanel pendingCount/mismatchBanner`、`exposed checkbox` 与 `spoof+headers`、`StatsPanel` 轮询与 `SupplierCreditsPanel percent`，`go test` 以 `httptest` 覆盖窗口与聚合及 `costUnknown`

## 实施总结
- 提交：`3960020` — `feat(rewrite-go-increment): webui sync and stats (#10)`
- 实现的 seams：S1 SWR 三键与 mutate, S2 乐观更新等待确认, S3 userAgent debounce, S4 Stats 轮询, S5 聚合全量重算与 credits, S6 formatCost
- 验收标准：7/7 全选
- 测试结果：go vet 通过；go test ./... 全绿；NODE_ENV=test npx vitest 26 files 219 tests 全绿
- typecheck：webui tsc --noEmit 通过
- 文档对齐：无 README 变更需求；目录干净
- 遗留 / 后续建议：S2 失败阻塞可加 Toast；SWR 未接 App 状态流仍以 getState 为确认源
