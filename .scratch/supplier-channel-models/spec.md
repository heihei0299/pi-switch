# 多渠道模型分区与网关二次注入

Status: ready-for-agent

## Problem Statement

当前供应商模型能力为供应商级扁平池：单 `Models[]` 与 `ExposedModels[]` 容纳全量模型，`POST /profiles/:name/fetch-models` 仅以首条上游（Primary）拉取 `/v1/models`，网关聚合亦仅汇聚该扁平暴露集。用户在同供应商下需接入多条渠道（多 key/多地域/多基址），期望每条渠道可独立发现新模型并按需暴露；网关侧则需从全部供应商/渠道的已暴露候选出发，按需二次勾选子集后再以 `supplier/channel/modelId` 唯一标识注入 `models.json: providers[prefix].models`。该意图未被产品化：无渠道主键与分区语义、无按渠道拉取/暴露口径、无网关二次勾选与前缀注入规则、无旧数据迁移定义。

## Solution

确立渠道（Channel）即上游（Upstream）、`name` 为同一供应商内唯一必填主键的领域语义；在上游上增量新增分区字段 `models` 与 `exposedModels` 实现按渠道隔离的模型池与暴露集，顶层 `models/exposedModels` 保留作迁移兼容；每渠道独立触发上游拉取与独立暴露（跨渠道同 id 视为独立条目）；网关侧改为本地聚合全量已暴露集（不做网络拉取），`id = supplier/channel/modelId` 前缀保证跨渠道唯一，并提供发布时瞬时二次勾选子集后原子注入 `models.json`；新渠道默认为空池，单渠道失败不阻断其他渠道，转发/failover/权重语义不改。

## User Stories

1. As an 供应商管理员, I want to 在单个供应商下添加多条渠道（Upstream）并以 `name` 作为渠道唯一主键, so that 同供应商可接入多 key/多基址
2. As an 供应商管理员, I want to 每条渠道拥有独立的模型池与已暴露集且跨渠道同 id 视为独立条目, so that 可按渠道区分定价与参数
3. As an 供应商管理员, I want to 对指定渠道独立触发“获取新模型”（拉取该渠道的 `/v1/models` 并按模型目录 enrich 后合并入该渠道池）, so that 新模型按渠道发现
4. As an 供应商管理员, I want to 每条渠道可独立勾选暴露模型且非法 id 被校验拒绝, so that 暴露按渠道精确控制
5. As an 供应商管理员, I want to 旧配置的顶层模型数据在首次保存时自动迁移至首条渠道且读取时优先分区、回退顶层, so that 无感升级不丢数据
6. As an 网关管理员, I want to 在网关预览中看到按供应商/渠道分组的已暴露候选聚合及前缀 id 视图, so that 明确待注入内容
7. As an 网关管理员, I want to 在网关发布面板对已暴露候选作二次勾选（按供应商/渠道/模型粒度）并以勾选子集预览 pending/差异, so that 可按需子集注入
8. As an 网关管理员, I want to 二次勾选为发布时的瞬时选择不持久化且发布为原子写 `models.json: providers[prefix].models`, so that 所见即所得且可重做
9. As an 网关管理员, I want to 已注入的网关模型 id 形如 `supplier/channel/modelId` 且跨渠道同名不冲突, so that 客户端可唯一定位
10. As an 用户, I want to 单渠道拉取失败仅影响该渠道且其他渠道仍可成功与提示, so that 批量拉取具容错
11. As an 用户, I want to 既有 `fetch-models` 与 `expose` 不带渠道参数时兼容回退至首条/主渠道以兼容旧客户端, so that 平滑兼容
12. As an WebUI 用户, I want to 在供应商编辑中按渠道分组查看/编辑模型与暴露勾选并得知来源渠道, so that 心智与存储一致
13. As an 开发者, I want to 通过集成测试验证分区存储、迁移、按渠道拉取/暴露、网关前缀聚合与二次勾选注入, so that 回归可护航

## Implementation Decisions

- 领域与术语：渠道为上游同义词，`name` 为同一供应商内唯一主键（必填、1-32 字符、仅 alphanum/`-`/`_`，`baseUrl` 仍校验 `http(s)` 但不作主键）；顶层供应商字段进入废弃期，日志与文档统一渠道/供应商/网关/网关发布词汇
- 数据模型：上游增量新增分区字段（每条渠道含模型池与已暴露集），顶层字段保留兼容；配置加载时优先分区、缺失回退顶层，保存时若检测到顶层非空且分区空则自动迁移至首条渠道并保留顶层只读镜像或清空以避免二次迁移歧义，新建渠道默认为空池
- 拉取语义：服务端按渠道参数定向对应上游的 `baseUrl/apiKey/headers` 发起拉取（兼容旧无参回退主上游），返回 id 列表按目录 enrich（复用既有模型目录映射与分字段覆盖规则：目录命中则覆盖 `cost/contextWindow/maxTokens/reasoning/input`、缺省补 `name`）后去重合并入该渠道池，不删用户已改条目；单渠道网络/解析失败仅记该渠道错误，不阻断其他渠道
- 暴露语义：暴露口按渠道分区校验（待暴露 id 必须存在于该渠道模型池），空 `exposedModels` 表示不暴露；服务端需校验渠道存在性、id 归属与跨渠道隔离，不做跨渠道去重
- 网关聚合与注入：网关合成视图构建改为遍历全部供应商/渠道的已暴露集作本地聚合（不做网络拉取），模型合成含 `id = supplier/channel/modelId`、透传 `contextWindow/maxTokens/cost/input/reasoning/name`；预览接口返回按供应商/渠道分组的已暴露候选与当前/提议差异及 pending 数，发布接口接收二次勾选子集（空子集即不注入）并原子写网关文件，失败语义与供应商侧隔离（ADR-0006）
- API 演进：`POST /profiles/:name/fetch-models` 新增可选渠道定向（查询串或 body 字段，旧无参保持兼容），`PUT /profiles/:name/expose` 新增渠道定向与分区校验，`GET /models/gateway/preview` 与 `PUT /models/gateway` / `POST /gateway/publish` 扩展为按渠道分组与勾选子集口径，`GET /v1/models`（代理侧）沿用已暴露聚合但 id 保持前缀形态；所有新增参数缺省均回退以兼容
- WebUI：供应商面板按渠道分组展示模型卡与暴露勾选、每渠道独立 Fetch 按钮与来源标识，网关面板按供应商/渠道分组展示已暴露候选并提供二次勾选与差异/待发布计数，文案与校验沿用现有吐司与表单形态
- 尊重既有 ADR：延续 ADR-0005 模型目录权威源与 ADR-0006 供应商与网关逻辑独立仅显式发布约束，本次不改转发/failover/权重路由

## Testing Decisions

- 仅验外部行为：以 Web API 集成为主 seam，辅以配置与网关模块单元及 WebUI 组件单测；不测内部调用计数
- 覆盖点（均按外部可观测判定）：
  - 渠道主键校验（`name` 必填、唯一、格式）与 `baseUrl` 校验
  - 配置迁移：旧顶层模型数据首次保存搬至首渠道、分区优先读取与回退、新增渠道空池
  - 按渠道拉取：定向渠道拉取 enrich 合并入该渠道池、跨渠道同 id 隔离、单渠道失败不穿透、旧无参回退主渠道
  - 按渠道暴露：分区校验（未知 id 400）、空暴露即不进网关、跨渠道独立
  - 网关聚合与注入：前缀 id `supplier/channel/modelId`、全量聚合正确、二次勾选子集注入与差异/pending 计数、发布原子写与回滚
  - 错误隔离：供应商写失败不影响网关预览/健康，反之亦然
- 复用既有测试形态：沿用服务端契约测试的 profile/网关用例、TUI 嵌入校验与 WebUI 模型/网关差异单测写法，使用 fixture 上游响应而非真实网络

## Out of Scope

- 不改变代理转发的运行时路径、failover 同模型择优与权重调度
- 不在网关发布时对每渠道实时网络拉取 `/v1/models` 再过滤（保持本地聚合）
- 不持久化网关二次勾选结果（瞬时选择，下次发布重选）
- 不提供按渠道的目录 provider 独立映射（沿用供应商级 `modelsDevProvider/preset` 推断）
- 不做旧 `models.json` 中 `supplier/modelId` 到 `supplier/channel/modelId` 的自动批量迁移脚本（需用户重新发布一次）
- 不引入独立网关进程/二进制与 IPC（延续 ADR-0006 逻辑隔离预留）

## Further Notes

- 约束来源：ADR-0006（供应商与网关逻辑独立）与 ADR-0008（渠道分区模型与网关二次勾选）及 CONTEXT.md 术语（供应商/渠道/上游/网关/网关发布）
- 缝（Seam）：复用既有最高缝——配置/服务端/网关聚合与 WebUI 供应商/网关面板，不新建顶层模块
- 迁移提示：旧配置升级后首次保存自动迁移，旧网关文件在下次发布后统一为前缀 id，新渠道创建后需手动 Fetch 填充
- 实现决议（code-review 回执，2026-09-06）：三段 id 精确 pin 到渠道（单候选，不跨供应商 failover）为 story 9 可调用所必需，不视为 Out of Scope 的转发变更；裸 id/二段 id 路由与 failover/权重语义不变
- 兼容决议：渠道名必填为新建语义，存量未命名渠道下次编辑需补名（前端先行校验，后端 400 兜底）；数据本身无损（保存拒绝不丢数）
- 读取语义：分区存在时空渠道读空（不回退顶层），保证跨渠道隔离；顶层残留仅在首渠道为空的保存时吸收
- 拉取语义：目录 enrich 只播种新增条目，不覆盖用户已调参数（用户编辑为源头）
- 子集 pending：发布面板勾选子集的待发布数本地计算（当前已注入 vs 勾选子集），服务端 pending_count 仍按全量暴露集
