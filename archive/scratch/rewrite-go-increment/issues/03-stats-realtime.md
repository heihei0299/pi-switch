## Question

实时统计的推送机制应如何设计？`StatsPanel` 的 `auto-refresh`（`5s/30s/5min`）与 `WebSocket`/`SSE` 的权衡，以及 `auto-refresh` 失败保留旧数据与窗口参数透传的细节？

需决策：
1. 推送：轮询 `GET /api/stats?window=...` 的 `5s/30s/5min` 四档（`Off` 默认，`setInterval`）vs `WebSocket`/`SSE` 的 `gin` 推送（`handleStatsStream`），`modernc/sqlite` 的 `requests` 表是否支持 `LISTEN/NOTIFY`
2. 窗口参数：`today|24h|7d|custom` 四档的 `from/to` 如何在轮询间透传，`auto-refresh` 失败是否保留旧数据与旧窗口
3. `handleGetCredits` 的 3 窗口（`rolling 20% / weekly 45% / monthly 70%`）的 `percent` 计算（`SupplierCreditsPanel` 的 `percent` undefined guard 已修）与 `cacheRate` 的实时一致性
4. `stats` 的 `byConversation` 与 `byModel` 的聚合口径（`okRequests` vs `total`）在实时推送下的增量更新 vs 全量重算

## Type

grilling

## Status

resolved

## Answer

1. **推送**：轮询 `5s/30s/5min` 四档（`Off` 默认，`setInterval`），`modernc/sqlite` 无 `LISTEN/NOTIFY`，不引入 `WebSocket`。
2. **窗口参数**：透传 `today|24h|7d|custom` 的 `from/to`，`auto-refresh` 失败保留旧数据与旧窗口。
3. **余量窗口**：`rolling 20% / weekly 45% / monthly 70%` 三档，`percent` 为已用/总量，`SupplierCreditsPanel` 的 `percent` undefined guard 已修。
4. **聚合口径**：每次轮询全量重算 `byConversation/byModel`（`okRequests` vs `total`），避免增量漂移。

## Blocked by

01
