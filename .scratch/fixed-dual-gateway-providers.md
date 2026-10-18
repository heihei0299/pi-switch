Status: ready-for-agent

# 固定双 provider 的网关聚合

## Problem Statement

当前网关发布按 Supplier/Channel 生成 `models.json` provider。两个 Supplier 下存在三个有效 Channel 时，客户端会看到三个 provider；真实上游数量、内部 Channel 名称和路由结构因此泄漏到客户端配置。

同时，Responses 与 Chat 的 API contract 被真实 Channel 数量间接决定，旧 provider 还可能残留；当多个 Channel 聚合时，session affinity、重复裸 model id、手动编辑字段和第三方 provider 的归属边界也不明确。

## Solution

网关发布只维护两个固定的 pi-switch gateway provider：

- `pi-switch-res`：固定声明 `openai-responses`；
- `pi-switch-chat`：固定声明 `openai-completions`。

当前 Supplier/Channel 仍是模型、凭证和实际路由的唯一事实来源。网关只将其 exposed models 按 Channel 的 effective API contract 聚合到对应的固定 provider；proxy 收到请求后，仍按裸 model id 找回真实 Supplier/Channel。

有 exposed model 时才输出对应 provider，因此最终是最多两个 pi-switch provider。非 pi-switch provider 可以继续存在并由用户维护。

## User Stories

1. As a pi-switch 用户，我希望网关只出现 `pi-switch-res` 和 `pi-switch-chat`，从而不因真实上游数量增加而产生更多客户端 provider。
2. As a Responses 客户端，我希望 `pi-switch-res` 声明 Responses API，从而按正确的 API contract 发起请求。
3. As a Chat 客户端，我希望 `pi-switch-chat` 声明 Chat Completions API，从而按正确的 API contract 发起请求。
4. As a pi-switch 用户，我希望 `openai-responses` Channel 的 exposed models 出现在 `pi-switch-res`，从而模型归属与上游能力一致。
5. As a pi-switch 用户，我希望 `openai-completions` Channel 的 exposed models 出现在 `pi-switch-chat`，从而多个 Chat 上游可以统一呈现。
6. As a pi-switch 用户，我希望不支持的 API 不生成第三 provider，从而网关管理范围保持固定。
7. As a pi-switch 用户，我希望不支持的 API 在 preview 中说明跳过原因，从而能区分“未暴露”和“不支持”。
8. As a pi-switch 用户，我希望模型 id 继续使用裸名，从而客户端不需要理解内部 Supplier/Channel 路由键。
9. As a pi-switch 用户，我希望所有 exposed model 的裸 id 全局唯一，从而 proxy 不会因入口不同而产生歧义路由。
10. As a pi-switch 用户，我希望重复裸 id 在 preview 中显示所有冲突来源，从而能准确修正模型暴露集。
11. As a pi-switch 用户，我希望重复裸 id 阻止整个发布，从而不会把部分正确、部分不可路由的配置写入 `models.json`。
12. As a pi-switch 用户，我希望发布失败时旧配置保持不变，从而不会因为一次错误编辑丢失可用网关。
13. As a pi-switch 用户，我希望两个 provider 的模型顺序稳定，从而内容未变化时不会产生无意义的 pending diff。
14. As a pi-switch 用户，我希望没有模型的 API contract 不生成空 provider，从而客户端不会看到不可用入口。
15. As a opencode 用户，我希望 opencode 来源模型单独携带 session affinity compat，从而上游能够收到所需的 session 信息。
16. As a 普通模型用户，我希望普通模型不继承 opencode 的 session affinity compat，从而不会给不需要的上游增加兼容行为。
17. As a pi-switch 用户，我希望修改 exposed models 后旧的 pi-switch provider 被清理，从而不会残留已撤销的模型和旧 Channel provider。
18. As a `models.json` 用户，我希望非 pi-switch provider 原样保留，从而网关发布不会破坏我手动维护的其他 provider。
19. As a pi-switch 用户，我希望固定 key 下已有的手动字段优先保留，从而不会被历史 provider 的迁移值覆盖。
20. As a pi-switch 用户，我希望旧 provider 中可按全局唯一 model id 识别的手动 model fields 得以迁移，从而升级后不必重复配置。
21. As a pi-switch 用户，我希望无法确定归属的旧 provider-level 字段被丢弃，从而不会把冲突的历史设置错误地施加到聚合 provider。
22. As a GatewayPanel 用户，我希望同时看到目标 provider 与来源 Supplier/Channel，从而既能理解最终配置，又能定位模型来源。
23. As a GatewayPanel 用户，我希望模型勾选仍作用于 exposed model 子集，从而可以控制本次发布的模型范围。
24. As a GatewayPanel 用户，我希望 JSON editor 继续直接可编辑并支持格式化，从而可以维护 gateway 中的非结构化字段。
25. As a GatewayPanel 用户，我希望可以维护第三方 provider，从而 pi-switch 不会限制整个 `models.json`。
26. As a GatewayPanel 用户，我希望新增第三个使用 pi-switch proxy 标识的 provider 会被拒绝，从而管理边界不会被绕过。
27. As a GatewayPanel 用户，我希望 preview 和后端 publish 都执行校验，从而其他调用方不能绕过 WebUI 规则。
28. As a proxy 用户，我希望通过 `/v1/responses` 使用 `pi-switch-res` 中的模型，从而请求能到达真实 Responses 或可转换的上游。
29. As a proxy 用户，我希望通过 `/v1/chat/completions` 使用 `pi-switch-chat` 中的模型，从而请求能到达真实 Chat 上游。
30. As a pi-switch 用户，我希望 Supplier/Channel 变更不会自动写入网关，从而网关发布仍是显式、可审计的动作。

## Implementation Decisions

- 保留现有 gateway 发布管线作为主要后端 seam，并在其上改变 provider 构建、校验、迁移和发布行为。
- 保留 GatewayPanel 作为主要前端 seam；展示层同时表达目标 provider 和来源 Supplier/Channel。
- provider key 固定为 `pi-switch-res` 与 `pi-switch-chat`，不再使用 Supplier/Channel 拼接 key 作为新的发布结果。
- provider API contract 固定：Responses 使用 `openai-responses`，Chat 使用 `openai-completions`。
- provider 的模型归属按 Channel 的 effective API contract 判断；`responsesMode` 继续决定 proxy 的透传/转换语义，不改变 gateway provider 分类。
- `anthropic-messages` 与 `google-generative-ai` 不进入两个固定 provider，并在 preview 中提供诊断。
- 模型 id 保持裸名；所有 exposed models 跨 Supplier/Channel 全局唯一。冲突时 preview 和 publish 均拒绝，发布保持原子性。
- provider 顺序固定为 Responses 后 Chat；各 provider 内模型按裸 id 字典序排序。
- 仅当聚合组至少包含一个有效 exposed model 时输出 provider。
- proxy 地址、proxy API key、provider 的基本运行字段保持现有 gateway contract。
- session affinity 不放在聚合 provider-level；来自 opencode.ai 的模型写入 model-level `sendSessionAffinityHeaders=true`，普通模型不生成该字段。
- 现有模型兼容字段继续保留；生成字段以当前 Supplier/Channel 配置为准。
- 发布清理旧的 pi-switch provider，保留非 pi-switch provider；固定 key 由 pi-switch 接管并覆盖。
- 旧 provider 的手动 model fields 按全局唯一 model id 迁移；固定 provider 已有字段优先；无法唯一归属的 provider-level 字段不迁移。
- JSON editor 仍可编辑第三方 provider，但 pi-switch 管理范围只能有两个固定 provider；第三个伪造 pi-switch provider 的请求由后端拒绝。
- 不改变 proxy 的裸 model id 路由机制；不通过入口 provider/API 对重复 id 做运行时消歧。
- 不自动发布；Supplier、Channel、设置或模型变更只产生新的 proposed gateway，必须显式应用。

## Testing Decisions

- Seam A 测试 gateway 发布管线的外部行为：给定 Supplier/Channel 配置、当前 gateway 和编辑内容，验证 proposed gateway、错误结果以及最终 `models.json` 内容。
- Seam B 测试 GatewayPanel 的用户行为：验证双层分组、模型勾选、固定 provider payload、preview 状态、JSON editor、格式化和失败保留编辑内容。
- 测试只验证外部行为，不绑定 map 遍历顺序、辅助函数名称或具体内部数据结构。
- 后端测试覆盖：三 Channel 聚合为两个 provider、单组省略空 provider、unsupported API 诊断、全局重复 id 原子失败、稳定排序、model-level compat、旧 provider 清理、第三方 provider 保留、手动字段迁移和固定 key 优先级。
- 前端测试覆盖：目标/来源双层展示、固定 provider 的发布载荷、第三方 provider 行为、伪造第三个 pi-switch provider 的错误显示、pending/removed/conflict 状态。
- 发布失败测试必须验证旧文件字节内容不变，而不只验证返回错误。
- 集成功能验证必须启动真实 proxy，分别通过 Responses 与 Chat 接口验证请求仍能路由到对应真实 Channel。
- WebUI 变更必须启动真实应用并在 browser 中验证最终 provider 数量、模型列表和 JSON editor 行为。

## Out of Scope

- 不改变 proxy 的 Supplier/Channel 路由算法、当前单候选行为、failover、weight 或请求转换规则。
- 不支持通过重复裸 model id 实现入口级路由消歧。
- 不为 Anthropic 或 Google API 增加第三个固定 provider，也不把它们强行伪装成 Chat。
- 不改变 Supplier/Channel 的模型池、凭证或 exposed model 持久化结构。
- 不将网关发布改为自动同步。
- 不删除或重写非 pi-switch provider。
- 不将 JSON editor 改造成只读结构化表单。
- 不修改历史请求日志、统计归属或模型目录来源。
- 不包含版本发布、npm 发布或 git tag 管理。

## Further Notes

- 本 spec 遵守 ADR-0010，并 supersede ADR-0009 关于“按 Channel 分 provider”的决定。
- 当前典型场景为两个 Supplier、三个 Channel；例如一个 Responses Channel 与两个 Chat Channel 最终只形成两个 gateway provider。
- “最多两个”指 pi-switch 管理的 provider；`models.json` 仍可同时包含用户维护的其他 provider。
- 首次从旧 provider 迁移到固定 key 时执行历史 model fields 迁移；后续以固定 key 的当前编辑结果为准。
- 固定 provider 的 API、proxy 地址和生成模型元数据由 pi-switch 维护；用户手动扩展字段遵循已确认的迁移与保留优先级。
- 该设计的完成标准是：真实 `models.json` 中 pi-switch 管理项最多为两个，两个接口均能在真实 proxy 中完成对应请求，且所有列出的失败场景都不会破坏旧配置。
