# 0008 渠道分区模型与网关二次勾选

日期: 2026-09-02
状态: 已接受
关联: ADR-0006（供应商与网关彻底逻辑独立，仅显式发布）

## 背景

供应商（Supplier）下已支持多上游 `upstreams[]` 用于同模型 failover，但模型发现与暴露仍是供应商级扁平池（`Models[]/ExposedModels[]`），仅首条上游参与 `/v1/models` 拉取；网关聚合亦仅汇聚该扁平暴露集。用户需在供应侧为每个渠道独立发现新模型并按需暴露，网关侧则在已暴露候选上二次勾选子集后再注入 `models.json: providers[prefix].models`，以 `supplier/channel/modelId` 保证跨渠道唯一。

## 决定

1. **术语**：渠道（Channel）即上游（Upstream）的同义词，`name` 为同一供应商内唯一必填主键，作为分区与网关前缀的稳定键；`baseUrl` 仍校验 `http(s)` 但不作主键。
2. **分区存储**：每条 Upstream 增量新增 `models: ModelEntry[]` 与 `exposedModels: string[]` 分区字段；顶层 `Supplier.models/exposedModels` 保留作迁移兼容，读取时优先分区、回退顶层，首次保存时旧顶层数据搬至首条渠道，其余渠道空池，新渠道默认为空池。
3. **按渠道隔离拉取与暴露**：每渠道可独立触发 `POST /profiles/:name/fetch-models?channel=name`（回退主渠道兼容旧调用），返回按 `models.dev` enrich 后去重合并入该渠道池，不删用户已改条目；跨渠道同 `id` 视为独立条目，暴露按渠道独立开关 `PUT /profiles/:name/expose` 按渠道分区校验。
4. **网关二次勾选与注入**：`BuildProposedGatewayEntry` 与 `handleGatewayPreview/Publish` 改为本地聚合全部供应商/渠道的已暴露集（不做网络拉取），`id = supplier/channel/modelId` 注入 `models.json`；GatewayPanel 在已暴露候选上提供按供应商/渠道/模型的二次勾选，勾选为发布时瞬时选择不持久化，发布失败不阻断供应商侧。
5. **不做范围**：本次不改转发/failover/权重/headers 路由语义，仅扩展数据模型与发布聚合口径。

## 备选

- 保持供应商级扁平池、多渠道去重共享：简化但无法按渠道区分定价/参数与独立暴露，否决。
- 在 Supplier 增平行 `channelModels` map 而不改 Upstream 结构：割裂连接与模型归属，查询与校验需双索引，否决。
- 网关侧实时对每渠道 `GET /v1/models` 再过滤：增加发布期网络依赖与上游下线误判，否决；改为本地聚合，二阶段校验留待后续。
- 以 `baseUrl` 或自增 `id` 作渠道主键：`baseUrl` 无法区分同地址多 key，`id` 需额外迁移生成；`name` 已为人可读且与现有 Upstream 字段一致，权衡后采用。

## 后果

- 正面：多密钥/多地域同供应商场景可按渠道独立演进模型池与暴露策略，网关二次勾选实现供应候选与最终注入的显式分离，`id` 前缀避免跨渠道重名。
- 负面：旧配置首次读写触发迁移，顶层字段进入废弃期；网关发布面板与服务端需新增按渠道分组与勾选逻辑，校验面扩大。
- 迁移：旧 `config.json` 的顶层模型数据自动搬至首渠道，无数据丢失；`models.json` 中旧 `supplier/modelId` 仍可读，发布后统一为 `supplier/channel/modelId` 前缀，已发布网关需重新发布一次。
