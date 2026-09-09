# 09: 网关与供应商端到端（合并）

**What to build:** 让网关发布与供应商管理在一次垂直切片内端到端可演示：`GatewayPanel` 的 `Current vs Proposed` 预览与 `PUT publish` 的原子落盘及 `merge_gateway_extra`，`ProfilesPanel` 的 `exposedModels/modelMap` 同步、`spoof` preset 与 `headers/Upstream` 合并、`Fetch from provider` 的真实 `GET /models + enrich` 与 `+Add` 全选防 `2 vs 3`，浏览器与 `curl` 均可验证 `models.json:providers[pi-switch]` 的写前空写后含 2 模型。

**Blocked by:** 08 API 合约与校验 responsesMode/validate

**Status:** resolved

- [x] `GET /api/models/gateway/preview → {current,proposed,conflicts,pending_count}`（`current=m models.json:providers[pi-switch]`，`proposed=BuildProposedGatewayEntry(supplier.exposedModels)`，`pending_count=added+removed+changed`），`GatewayPanel` 首进 `pendingCount>0` 时 `showMismatchBanner` 且不自动 `PUT`，`diffGateway` 的 `added/removed/changed` 可断言
- [x] `PUT /api/models/gateway`（兼容 `PUT /api/gateway/publish`）校验 `api∈SUPPORTED_APIS && baseUrl http(s) && models[].id` 后 `backup_models + write_models_atomic(tmp-⟨pid⟩→models.json) + notify_gateway_changed`，失败不 reload；`proposed.models` 仅含 `exposedModels` 引用（未命中以 `id+默认 input/contextWindow/maxTokens` 兜底），`merge_gateway_extra` 保留 `current` 非生成键与同 `id` 模型的 `headers/compat/extra`；成功同步 `settings.gatewayApi/baseUrl→proxy.host/port（0.0.0.0→127.0.0.1）` 并 `mutate ["gateway"]+["profiles"]`
- [x] 写前 `providers == {} || missing pi-switch`，写后 `providers[pi-switch]` 含 2 模型（`id="{supplier}/{modelId}", reasoning→compat.supportsDeveloperRole=false`），回归 `multi_supplier 3 vs 2`
- [x] `ProfilesPanel: ModelCard checkbox ↔ PUT /api/profiles/:name/expose {modelIds}`（校验 `exposedModels` 每项命中 `models[].id`），`modelMap` 仅透传（`StructuredOptionsEditor` 编辑，不参与 `proposed` 生成）
- [x] `spoof(rename="userAgent") ∈ {"",claude-code,codex,gemini} → PUT /api/profiles/:name/spoof`，全局回退 `Settings.proxy.userAgent`，转发 `User-Agent = spoof || proxy.userAgent || "curl/8.5.0"`，`X-Custom` 经 `headers/Upstream.headers`（`resolvedUpstreams()`，`Upstream.headers > Profile.headers`）合并可观测
- [x] `POST /api/profiles/:name/fetch-models → GET <primary_baseUrl>/models|/v1/models + Bearer + 5s timeout`，遍历 `data[].id` 与 `models[]`，`defaultModel(id)` 赋 `input=["text"] contextWindow=128000 maxTokens=16384`，经 `enrich_models_with_catalog_with_stats(resolve_models_dev_provider)` 分字段覆盖（`cost/limit/reasoning/modalities.input`，`name` 缺省补齐，`thinkingLevelMap/headers/compat` 不覆盖），`failed=len+warning / skipped=len` 的 `EnrichStats` toast，`loading disabled` 与 `error` 保留本地
- [x] `+Add` 必选 `preset`（`get_presets`），新建 `exposedModels` 默认全选全部 `models[].id`（可逐项取消），`validateProfileJson/validate_provider_profile` 前置校验 `baseUrl http(s) / duplicate id / exposedModels引用`，`modelsDevProvider unknown` 仅 `warning` 不阻塞
- [x] 最高缝 `httptest.Server` mock 上游的 `GET /models`（`Bearer`）、`publish` 写前写后差异与 `ss -tlnp` 无关的前端 `vitest` 的 `GatewayPanel` 与 `ProfilesPanel` 覆盖

## 实施总结
- 提交：`c936c61` — `feat(rewrite-go-increment): gateway supplier end-to-end (#09)`
- 实现的 seams：S1 preview pending, S2 publish 原子写与 merge_extra, S3 写前空写后2模型, S4 exposedModels/modelMap, S5 spoof/disguise, S6 fetchModels enrich, S7 +Add 全选
- 验收标准：8/8 全选
- 测试结果：go test -run TestGatewaySupplier_09 7/7 全绿；go test ./... 全绿；go vet 通过
- typecheck：通过
- 文档对齐：internal/config 新增 Reasoning/proxy.userAgent；gateway.go 新增 Diff/Merge/compat；server.go 同步 gatewayApi/host/port
- 遗留 / 后续建议：models.dev catalog 为静态 stub；前端 SWR mutate 依赖未在 Go 端额外校验
