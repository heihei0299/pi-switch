## Question

WebUI 的状态同步策略应如何设计？`SWR`/`React Query` 的缓存与乐观更新何时失效，尤其在 `gateway publish` 后 `models.json` 的即时一致性与 `ProfilesPanel` 的 `exposedModels/modelMap` 同步上？

需决策：
1. 数据抓取：`SWR` vs `React Query` vs 手动 `useEffect+fetch`，缓存键（`["profiles"]`, `["gateway"]`, `["stats", window]`）与失效时点（`mutate` after `PUT /api/profiles|gateway/publish`）
2. 乐观更新：`PUT /api/profiles/:name` 后是否乐观更新本地 `profiles` 列表，还是等待 `GET /api/profiles` 确认
3. 网关一致性：`GatewayPanel` 的预编辑（`preview pending`）与直接写盘的差异，`publish` 后 `models.json:providers[pi-switch]` 的写前空与写后含模型的差异如何即时反映到 `ProfilesPanel` 的 `exposedModels` 选中态
4. `userAgent` disguise 的实时预览：`ProfilesPanel` 的 `userAgent` 输入与 `provider` 的 `headers` 的联动，是否需要 `debounce` 与 `preview` 态

## Type

grilling

## Status

resolved

## Answer

1. **数据抓取**：`SWR`，缓存键 `["profiles"]`/`["gateway"]`/`["stats", window]`，`mutate` after `PUT /api/profiles|gateway/publish`。
2. **乐观更新**：等待 `GET /api/profiles` 确认后再更新，避免与 `gateway publish` 的 `exposedModels` 竞争。
3. **网关一致性**：`GatewayPanel` 的 `preview pending` 的 `diff` 预览，`publish` 后 `mutate ["gateway"]` 与 `["profiles"]`，使 `ProfilesPanel` 的 `exposedModels` 即时一致。
4. **userAgent 预览**：`debounce 300ms` 后 `preview` `headers` 的 `X-Custom` 合并，`Upstream` 聚合时可见。

## Blocked by

01
