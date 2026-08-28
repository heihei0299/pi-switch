# 0005 — 以 models.dev 目录作为模型元数据权威源

上游 `/v1/models` 仅返回模型 id，无 pricing / limit / reasoning 等元数据，无法直接产出可用的 ModelEntry。决定：所有“发现/更新模型”路径（Fetch Models 等）以 https://models.dev 的 `api.json` 目录为权威元数据源，按 `limit.context/output→contextWindow/maxTokens`、`cost→cost`、`reasoning`、`modalities.input→input` 的映射 enrich 本地模型；目录未覆盖的 id 保留原值，目录新增的模型仅在用户勾选后新增。目录全量 4.4MB，本地缓存于 `~/.pi-switch/cache/models-dev.json` 并以 24h TTL 刷新，失败时降级到上游或缓存。
