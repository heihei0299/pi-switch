## Question

网关与供应商管理的 UI 交互细节应如何设计？`GatewayPanel` 的预编辑 vs 直接写盘、`ProfilesPanel` 的 `exposedModels/modelMap` 同步与 `userAgent` disguise 实时预览，以及 `handleFetchModels` 真实 `GET <baseUrl>/models` 的 enrich 交互？

需决策：
1. 网关：`GatewayPanel` 的 `preview pending`（`PUT /api/gateway/publish` 前的 `diff` 预览）vs 直接写盘，`publish` 后 `models.json:providers[pi-switch]` 的 `api`/`models` 的 `exposedModels` 过滤与 `modelMap` 的覆盖策略
2. 供应商：`ProfilesPanel` 的 `exposedModels` 多选与 `modelMap` 的 `provider`/`model` 覆盖，`userAgent` 的 `preset`（`claude-code/codex/gemini`）与 `headers` 的联动，`disguise` 的 `X-Custom` 透传在 `Upstream` 聚合中的 `headers` 合并
3. 获取模型：`handleFetchModels` 的真实 `GET <baseUrl>/models`（`Authorization: Bearer <apiKey>`）的 `enrich`（`cost/limit/reasoning`）与本地 `ModelEntry` 的合并策略（`cost` 覆盖 vs 保留）
4. 新供应商：`+Add` 的 `preset` 选择与 `Fetch from provider` 的 `loading/error` 态，`new-provider` 残留的 `2 vs 3` 模型问题的 `exposedModels` 默认全选 vs 空

## Type

grilling

## Status

resolved

## Blocked by

01

## Answer

**决策总览**：延续 `feat/rewrite-go@2f113a1` 已交付的 `preview pending` 交互与 `2-model` 契约，不引入新抽象；Go 重写仅对齐 `gateway.rs:preview_gateway/apply_gateway/build_proposed_gateway_entry` 与现有 `GatewayPanel.tsx/ProfilesPanel.tsx` 的状态流，保持 `types.ts` 手写同步（决策 01）与 `SWR mutate ["gateway"]+["profiles"]`（决策 02）一致。

1. **网关 GatewayPanel：`preview pending` 差分预览，不直接写盘**
   - 状态源：`GET /api/models/gateway/preview` → `{ current, proposed, conflicts, pending_count }`（`gateway.rs:preview_gateway`）。`current` 为 `models.json:providers[pi-switch]` 现存条目（`None` 视为 `{}`），`proposed` 由 `build_proposed_gateway_entry` 从 `config.json:profiles[*].exposedModels` 聚合生成（`api=gatewayApi, baseUrl=http://<host>:<port>/v1, apiKey=pi-switch-proxy, models=[{id:"{supplier}/{modelId}", ...ModelEntry}]`，`reasoning` 模型补 `compat.supportsDeveloperRole=false`），`pending_count` 经 `compute_pending_count`（`added+removed+changed`）。
   - 前端状态：`previewGateway()` load 后存 `current/proposed/conflicts/backendPending`，`statusDiff=diffGateway(current, proposed)`（`gatewayDiff.ts`），`pendingCount = backendPending ?? diff size`；首次进入若 `pendingCount>0` 置 `showMismatchBanner=true`（“Current vs Proposed 待发布提示”），默认不自动 `PUT`。
   - 编辑与校验：`draft/modelsPreview` 由 `ModelDraft` 合成 `liveJson`，`validateGatewayJson` 校验；`PUT /api/models/gateway` 入参 `edited: Value` 经 `validate_gateway_value`（`api/baseUrl/models[].id` 必填，`api ∈ SUPPORTED_APIS`），写前 `backup_models`、原子写 `models.json.tmp-⟨pid⟩→models.json`（`write_models_atomic`）、`notify_gateway_changed`，失败不 reload。
   - 过滤与覆盖：`proposed.models` 仅含 `exposedModels` 引用的模型（未命中 `ModelEntry` 时以 `id` + 默认 `input/contextWindow/maxTokens` 兜底）；`merge_gateway_extra` 将 `current` 中非生成键（`generated_keys=[api,baseUrl,apiKey,models,proxy]` 外）与同 `id` 模型的额外字段（`headers/compat/extra`）合并入 `proposed`，保证二次发布不丢自定义 `headers/compat`。`api`/`models` 的 `exposedModels` 过滤是唯一真源，`modelMap` 不参与 `proposed` 生成（见 2）。
   - 发布后联动：`handleApplyToPi` 成功后同步 `settings`（`gatewayApi ← payload.api`，`proxy.host/port ← payload.baseUrl` 的 `hostname/port`，`0.0.0.0/[::]` 归一为 `127.0.0.1`），`localStorage[LAST_PUBLISH_KEY]=now`，`toast ok`，`await load()` + `await refresh()`（上游 `SWR mutate ["gateway"]` 与 `["profiles"]` 使 `ProfilesPanel` 的 `exposed` 徽标即时一致）；失败保留 `draft` 不 reload，`toast err`。

2. **供应商 ProfilesPanel：`exposedModels` 主真源，`modelMap` 透传保留，`userAgent` preset 与 `headers` 联动**
   - `exposedModels`：`ProviderProfile.exposedModels: string[]`（`config.rs`）为网关收录唯一依据；UI `ModelCard` 的 `exposed checkbox` 逐项 `onToggleExposed` 增删该数组（`PUT /api/profiles/:name/expose { modelIds }` → `ops::update_exposed_models`），校验 `exposedModels` 每项必须命中 `models[].id`（`validate_provider_profile`），`3 vs 2` 回归断言：写前 `providers:{}` 或缺 `pi-switch`，写后 `providers[pi-switch].models` 含 2 模型（与 Issue 01 契约一致）。
   - `modelMap`：保留 `ProviderProfile.modelMap?: Map<string, Value>` 的透传语义，不参与网关 `proposed` 构建；若需 pi 侧别名（`{ "alias": "real/model" }`），前端 `StructuredOptionsEditor` 编辑，后端 `#[serde(flatten)] extra`+显式 `model_map` 均写回，未来路由层可选消费，当前仅校验 `key 非空`。避免 `exposedModels` 与 `modelMap` 双真源竞争。
   - `userAgent`/`disguise`：`Profile.spoof?: string`（`rename="userAgent"`）枚举 `"" | "claude-code" | "codex" | "gemini"`（`SPOOFS`），`Select` 绑定 `PUT /api/profiles/:name/spoof { spoof }`；全局回退 `Settings.proxy.userAgent`（`settings.proxy.userAgent`）。`disguise` 的 `X-Custom` 等自定义头经 `headers?: Map<string,Value>` 与 `Upstream.headers` 聚合：`resolvedUpstreams()` 时 `upstreams` 非空取 `Upstream`，否则回退单 `baseUrl/apiKey/headers`；`RequestHeadersEditor` 编辑 `profile.headers`，`UpstreamEditor` 编辑各 `Upstream.headers`；转发时 `build_upstream_headers` 按 `Upstream.headers > Profile.headers` 合并，`User-Agent` 由 `spoof || settings.proxy.userAgent || DEFAULT_FORWARD_USER_AGENT ("curl/8.5.0")` 决定（与 `proxy.rs:forward` 与 `config.proxy.userAgent` 热生效一致），`X-Custom` 等 `disguise` 头透传可观测于 `stats` 的上游聚合。
   - 实时预览：`debounce 300ms` 后 `modelPreview(draft)` 合成 `preview headers`（与 Issue 02 的 `preview pending` 复用），不自动写盘，仅 UI 高亮。

3. **获取模型 `handleFetchModels`：真实 `GET <baseUrl>/models` + 模型目录 enrich，按分字段覆盖**
   - 触发：`ProfilesPanel` 的 `Fetch from provider` 按钮 `POST /api/profiles/:name/fetch-models`；后端以 `primary_base_url/primary_api_key`（`ProviderProfile::primary_*`，多 `Upstream` 取首条）发 `GET <baseUrl>/models`（`Authorization: Bearer <apiKey>`），返回 `{ models: string[] }`，前端合成 `ModelEntry[]`（`defaultModel(id)` 赋 `input=["text"], contextWindow=128000, maxTokens=16384`）。
   - Enrich：`ops::enrich_models_with_catalog_with_stats` 按 `resolve_models_dev_provider`（`modelsDevProvider` 显式值 > `preset→modelsDevProvider` 推断 `preset_to_models_dev_key`）取 `catalog[providerKey].models`，分字段覆盖：`cost.input/output/cacheRead/cacheWrite`、`limit.context → contextWindow / limit.output → maxTokens`、`reasoning`、`modalities.input → input`、`name` 仅缺省时补齐；`thinkingLevelMap/headers/compat/extra` 不覆盖；`cost.tiers` 忽略（仅外层）。缺 `catalog`（`None`）时 `failed = len` 并带 `warning`，`provider` 未命中时 `skipped = len`；前端 `toast` 展示 `EnrichStats{ enriched/skipped/failed, warning }`（与 `update_provider_models_with_stats` 日志一致）。
   - 交互：`loading` 时按钮 `disabled` + `spinner`，`error` 时 `toast err` 保留本地 `models`，成功后 `PUT /api/profiles/:name/models { models }` 写入，`EnrichStats` 可观测，`SWR mutate ["profiles"]`。

4. **新供应商 `+Add`：preset 必选，`exposedModels` 默认全选，`new-provider` 残留已清**
   - `+Add` 弹窗：`preset` 必选（`get_presets` 列表，`API_TYPE_OPTIONS` 校验 `api ∈ SUPPORTED_APIS`，`baseUrl` 必 `http(s)://`），填充 `preset.baseUrl/api/models` 模板；`Fetch from provider` 提供 `loading/error` 态（`useAction` 的 `pending` + `toast`），`Fetch` 成功 enrich 后 `modelIds` 预填。
   - `exposedModels` 默认：新建时 `models` 的全部 `id` 自动写入 `exposedModels`（全选），避免 `2 vs 3` 的 `new-provider` 残留（曾因默认空导致 `providers[pi-switch]` 仅 2 而 `profiles[new-provider]` 有 3）；用户可在 `ModelCard checkbox` 逐项取消，允许空（`exposedModels=[]` 时 `proposed.models=[]`，网关 `pending_count` 反映 `removed`）。
   - 校验：`validateProfileJson`/`validateProviderProfile` 在 `POST /profiles` 与 `PUT /profiles/:name` 前置校验（`baseUrl/api/apiKey`、`duplicate model id`、`exposedModels` 引用校验、`modelsDevProvider` 未知 `warning` 级 `ValidationIssue` 不阻塞保存，与 Issue 01 的“仅提示不阻塞”一致），`2 vs 3` 回归由 `gateway preview` 的 `pending_count` 与契约测试断言兜底。

   **领域术语对齐**（`CONTEXT.md`）：供应商=ProviderProfile（模型与凭证唯一真源）、上游=Upstream（`baseUrl/apiKey/headers/weight/name`，多条聚合首条为网关主上游）、网关=写入 `models.json:providers[pi-switch]` 的合成视图、网关发布=显式 `PUT /models/gateway` 唯一落盘路径（发布外供应商变更不自动写网关）、余量= credits 三窗口 `rolling 20% / weekly 45% / monthly 70%`（与本票无关，保留）。

   **可测试性**（供 `tdd-implement` 走最高缝 `go test ./...` + `NODE_ENV=test vitest`）：`preview pending` 的 `pending_count`、`merge_gateway_extra` 字段保留、`exposedModels` 引用校验与 `2 vs 3` 写前后断言、`handleFetchModels` 的 `httptest.Server` enrich 与 `EnrichStats`、`spoof/headers` 合并与 `DEFAULT_FORWARD_USER_AGENT` 兜底均为外部可观测行为。
