# Spec: 渠道级 API 隔离（复刻 CLIProxyAPI 模型级路由，支撑同供应商混布 chat/responses）

Status: ready-for-agent
Version: 1.0
Author: pi-switch maintainers
Date: 2026-09-07
Scope: 配置 `Supplier/Channel` 数据面、代理 `PlanRequest` 路由、网关聚合与 WebUI（不含 per-conversation 熔断）

## Problem Statement

用户在 `oc` 供应商（`https://opencode.ai/zen/go/v1`）下同时暴露 `mimo-v2.5`（chat-only）与 `muse-spark-1.2-contributor`（responses-only）等模型，经网关 `pi-switch` 请求时 `POST /v1/chat/completions {"model":"oc/mimo-v2.5"}` 与 `POST /v1/responses {"model":"oc/mimo-v2.5"}` 之一必 `500 Internal server error`。直连上游验证：`mimo` 仅 `chat` 通（`responses` 500），`muse-spark` 仅 `responses` 通（`chat` 500），单 `Supplier` 单 `api=openai-responses` 无法表达混布，`PlanRequest` 按 `profile.api` 选路导致错转。

## Solution

复刻 `router-for-me/CLIProxyAPI` 的“模型级 `APIBackend` + 同域多实例”隔离：为 `Channel`（即 `Upstream`）新增 `api`/`responsesMode`，`oc` 按模型能力拆两 `Channel`（`oc/chat` 与 `oc/responses`），代理按 `Channel.api` 而非 `Profile.api` 定 `PlanRequest` 的 `UpstreamPath` 与转换，`supplier/channel/model` 精确 pin 已有语义保持，短历史 `chat` 走 `chat`、`responses` 走 `responses`，各取所需。

## User Stories

1. As a pi 用户，我用 `oc/mimo-v2.5` 发 `chat` 请求时，代理经 `oc/chat` 的 `openai-completions` 直通 `/v1/chat/completions` 且 200
2. As a pi 用户，我用 `oc/muse-spark-1.2-contributor` 发 `responses` 请求时，代理经 `oc/responses` 的 `openai-responses` 透传 `/v1/responses` 且 200
3. As a pi 用户，我经网关 `pi-switch` 选 `oc/chat/mimo-v2.5` 与 `oc/responses/muse-spark` 时，`models.json` 的 `providers[pi-switch].models` 正确前缀且可分别选中发布
4. As a pi 用户，我在 WebUI 的 `Profiles → oc → Channels` 中能为每个渠道选择 `API`（`openai-completions/openai-responses/anthropic-messages`）与 `ResponsesMode`，保存后即生效
5. As a pi 用户，我的存量 `config.json`（单 `oc` 无 `api` 分区）首次保存时自动按模型能力归类到两 `Channel`，不丢 `models/exposedModels`
6. As a pi 用户，我用 `oc/mimo-v2.5` 发 `responses`（或 `muse-spark` 发 `chat`）时，代理能按渠道 `api` 做正确转换（`chat↔responses`）而非错转 500
7. As a 开发者，我能通过 `GET /v1/models` 看到 `oc/chat/mimo-v2.5` 与 `oc/responses/muse-spark` 的 `owned_by` 区分
8. As a 开发者，我在 `api=openai-responses` 渠道上发 `chat` 时，校验 `responsesMode` 与 `CLIProxyAPI` 一致（`passthrough` 才允许 `chat→responses`）
9. As a 运维，我能通过 `GET /api/validate` 看到渠道级 `api` 非法时的 `error` 提示
10. As a 运维，我回滚旧版本时，未分区配置仍可读（`HasChannelPartitions=false` 回退顶层）

## Implementation Decisions

- **数据面**：`Upstream` 新增 `api?: string`（`openai-completions|openai-responses|anthropic-messages`）与 `responsesMode?: string`（`auto|passthrough|convert`），与 `Profile` 同枚举，缺省继承 `Profile.api/responsesMode`；`ModelEntry` 不新增 `api`，能力归 `Channel`。`MigrateTopLevelToFirstChannel` 兼容：旧单 `Profile` 无分区时，首次按 `models` 启发式拆分到两 `Channel`（或保持单 `Channel` 由用户手动拆）。
- **路由**：`resolveRoute` 保持 `supplier/channel/model` 精确 pin 与 `supplier/model` 单候选，不引入新候选；`narrowToChannel` 保留 `api`，`profForAttempt` 返回窄化后的 `Profile`，`PlanRequest` 的 `api/mode` 取窄化后 `prof.API/prof.ResponsesMode`（即 `Channel` 覆盖），`UpstreamPath` 由 `Plan.To` 决定，`clampBody` 的 `ModelEntry` 取 `ChannelView`。
- **网关**：`BuildProposedGatewayEntry` 与 `Preview` 聚合保持 `supplier/channel/model` 前缀，`owned_by` 按 `Channel` 区分，二次勾选不变。
- **校验**：`validateRetryFields` 旁新增 `validateUpstreamAPI`（`api` 合法性与 `responsesMode` 兼容性复用 `ValidateResponsesMode`），`handleValidate` 产 `level:error`。
- **兼容**：`LoadConfigAtPath` 读时 `api` 缺省回退 `Profile.api`，`MigratedForSave` 不删分区；`DefaultConfig` 仍单 `test-provider`。
- **不引入 `ModelInfo` 全量注册表**：复刻 `CLIProxyAPI` 的“按模型选靶”语义，但以 `Channel.api` 为载体而非全局 `registry`，保持 `pi-switch` 现有 `supplier/channel` 心智。

## Testing Decisions

- 只测外部行为，不测内部遍历顺序；`retry.go` 保持休眠，不测冷却。
- **Seams（最高可测面，1-2 个）**：
  - `S1 配置`：`LoadConfigAtPath/MigratedForSave` 对含 `upstreams[].api` 的新旧配置读写
  - `S2 代理`：`POST /v1/chat/completions` 与 `POST /v1/responses` 对 `oc/chat/mimo` 与 `oc/responses/muse-spark` 的分流直通（`httptest` 上游区分 `chat`/`responses` 路径）
- **先验**：`internal/proxy/limit_test.go` 的 `Clamp`、`internal/server/channel_*_test.go` 的分区与网关二次勾选为同类分区测试范式
- **新增**：
  - `channel_api_routing_test.go`：`chat` 打 `mimo` 经 `chat` 通道 200，`responses` 打 `muse-spark` 经 `responses` 通道 200；错配（`chat` 打 `responses` 通道模型）按 `Plan` 转换后仍 200
  - `config_channel_api_test.go`：旧配置无 `api` 可加载，保存后分区保留 `api`，`validate` 对非法 `api` 报 `error`

## Out of Scope

- `per-conversation` 熔断/限流（`retry.go` 保持休眠）
- 引入 `tiktoken` 或模型专属分词器
- `CLIProxyAPI` 的全局 `ModelInfo` 注册表与插件路由全量复刻
- `TUI` 的渠道 `api` 编辑（WebUI 先行，TUI 仅只读展示）
- 对 `input` 超窗的 `compaction`（`pi` 侧负责）

## Further Notes

- 本 spec 复刻 `CLIProxyAPI` 的 `internal/registry` 模型级 `APIBackend` 与 `translator/registry.go` 的 `Register(chat↔responses)` 分表思想，但以 `Channel.api` 轻量化落地，不引入 `pluginapi.ModelRouteRequest`。
- 存量 `oc` 的 `500` 可通过本 spec 的 `S2` 直接复现：`chat` 打 `mimo` 经 `responses` 通道即 500，直连上游 `chat` 则 200。
- 不新增 `ADR`：`Channel` 分区已在 `ADR 0008` 确立，本次仅为 `api` 维度扩展，可逆。
