# Spec: Chat↔Responses 双向归一化与短模型 ID 别名

Status: ready-for-agent

## Problem Statement

用户在使用 `pi` 通过 `pi-switch` 代理访问 `oc` 供应商时，模型 `oc/chat/mimo-v2.5` 在 `/v1/chat/completions` 下直连正常，但经由 `/v1/responses` 网关（`oc/chat/mimo-v2.5` + `response` 接口）必现 `400 Invalid request parameters`（非流式）或 `500 Internal server error`（流式）。已定位为 `Responses→Chat` 转换未归一化 `input_text/input_image` 类型，直接透传 `input_text` 给要求 `text` 的上游 `Chat Completions` 导致上游校验失败。

对称地，`Chat→Responses` 方向同样存在直拷 `content` 的问题：`chat` 的 `text`/`image_url` 未转为 `Responses` 要求的 `input_text`/`input_image`，`oc/responses/muse-*` 经 `/v1/chat/completions` 的跨通道调用在携带图片或复杂内容时存在同类 400 风险。

同时，当前路由要求全限定三段式 `supplier/channel/model`（如 `oc/responses/muse-spark-1.2-c`），用户期望简化为两段式 `supplier/model`（如 `oc/muse-spark-1.2-c`），由模型反推渠道，减少记忆与配置成本。现有两段式仅查主通道，对已分区的供应商会误判 `no_route`。

## Solution

1. **补齐双向内容归一化**：`Responses→Chat` 已修复 `input_text→text`、`input_image→image_url`、`output_text→text`；本次补齐反向 `Chat→Responses` 的 `text→input_text`、`image_url→input_image` 归一化，保持 `string` 内容的兼容包装，`system` 仍抽为 `instructions`，`tools` 保持直拷（暂不引入命名空间复杂分支）。

2. **短 ID 别名路由**：保留三段式全限定兼容，新增两段式 `supplier/model` 别名解析——若两段式未在顶层命中，则遍历该供应商全部分区的已暴露集定位模型所在渠道，自动窄化到该渠道并按其 `api/responsesMode` 决定转换路径；`/v1/models` 同时暴露短别名，歧义（同供应商内同名跨渠道）时返回明确的 `ambiguous` 指引要求使用三段式。

用户视角：`oc/muse-spark-1.2-c` 与 `oc/responses/muse-spark-1.2-c` 均可在任意网关接口（`chat`/`responses`）下互转成功；`oc/chat/mimo-v2.5` 经 `responses` 也不再 400/500。

## User Stories

1. As a `pi` 用户，我想用 `oc/chat/mimo-v2.5` 通过 `/v1/responses` 发送 `input_text` 消息，以便在 `pi` 默认 `responses` 网关下无需改模型配置即可对话
2. As a `pi` 用户，我想用 `oc/chat/mimo-v2.5` 通过 `/v1/responses` 流式对话，以便获得与 `/v1/chat/completions` 一致的增量体验
3. As a `pi` 用户，我想用 `oc/responses/muse-*` 通过 `/v1/chat/completions` 发送文本，以便在 `chat` 网关下复用同一模型
4. As a `pi` 用户，我想用 `oc/responses/muse-*` 通过 `/v1/chat/completions` 发送带图片的内容，以便多模态输入在跨接口转换后仍被上游识别
5. As a `pi` 用户，我想用两段式 `oc/muse-spark-1.2-c` 代替三段式 `oc/responses/muse-spark-1.2-c`，以便减少模型 ID 长度与记忆成本
6. As a `pi` 用户，我想用两段式 `oc/mimo-v2.5` 调用任意接口，以便 `chat` 渠道模型也能短写
7. As a 代理运维者，我想让三段式全限定继续可用，以便存量配置与网关发布不受破坏
8. As a 代理运维者，我想在同供应商内同名模型歧义时收到明确错误指引，以便知道需改用三段式消歧
9. As a 开发者，我想让翻译器对未知 `type` 透传，以便未来新增内容类型不被静默丢弃
10. As a 开发者，我想让 `string` 类型的 `content` 在 `Chat→Responses` 时自动包装为 `input_text` 数组，以便兼容 `pi` 的简写消息
11. As a 测试者，我想让跨通道转换在 `mock` 上游中按路径断言成功，以便在不依赖真实上游时验证路由与转换
12. As a 网关用户，我想在 `/v1/models` 同时看到短别名与全限定，以便在模型选择器中按短名搜索

## Implementation Decisions

- **翻译器归一化对称**：在翻译器模块新增与既有 `normalizeResponsesContent` 对称的 `normalizeChatContent`，负责 `Chat→Responses` 的内容类型映射；`Responses→Chat` 的归一化保留，已处理 `input_text`、`input_image`、`output_text`。`Anthropic` 路径不改。
- **Chat→Responses 映射规则**：`string` → `[{type:"input_text",text}]`；数组中 `text` → `input_text`，`image_url:{url}` → `input_image:{image_url}`，`input_text/input_image` 已是目标形态则透传；`output_text` 不应在 Chat 侧出现，若出现则按 `text` 处理；未知 `type` 透传。
- **路由别名策略**：路由解析模块的二段式分支增加跨分区兜底搜索，命中唯一渠道时自动窄化并返回 `pinnedChannel`；三段式路径保持最高优先级。歧义（多渠道同名）时返回 `ambiguous` 错误而非随机选择。
- **模型暴露**：模型目录聚合时为每个已暴露模型额外生成短别名条目，短别名与全限定指向同一底层模型条目，`owned_by` 保持供应商标识。
- **接口契约**：`POST /v1/chat/completions`、`POST /v1/responses`、`GET /v1/models` 行为保持兼容；新增短别名不改变既有三段式的请求与响应形状；错误类型新增 `ambiguous` 仅用于两段式歧义场景。
- **架构约束**：不引入 `gjson/sjson`，沿用现有 `map[string]interface{}` + `encoding/json` 风格，避免引入新的运行时依赖与许可风险；`tools/reasoning/namespace` 的全量对齐不在本次范围，`tools` 保持直拷。
- **术语一致性**：沿用项目词汇表中的供应商、渠道、上游、网关、Responses 透传模式等定义撰写文档与错误文案。

## Testing Decisions

- **好的测试标准**：仅测外部可观察行为（HTTP 状态、响应 `object`/`output` 形态、上游命中路径），不测内部 `map` 构造细节；每个转换方向用最小 `input`/`messages` 载荷验证类型映射正确性。
- **复用现有最高可用缝**：
  - 翻译器单元测试缝（`TestRegistry_*`、`reasoning_test`）：新增 `Responses→Chat` 与 `Chat→Responses` 的内容归一化用例，覆盖 `input_text`、`input_image`、`string` 三类输入
  - 服务集成测试缝（`channel_api_routing_test.go` 的 `httptest` + `NewProxyRouter`）：复用 `channelAPIMock` 按路径断言，新增 `oc/mimo` 与 `oc/muse` 的二段式跨通道用例，以及 `image_url` 跨转用例；现有三段式用例保持绿
- **新增回归**：为已修复的 `oc/chat/mimo-v2.5` 经 `responses` 的 `400/500` 增加非流式与流式两条回归用例，断言修复后 `200` 且 `output` 含 `output_text`
- **先验参考**：`TestChannelAPI_CrossConversion` 已验证跨通道 `responses→chat` 的路径选择，本次在其上扩展短别名与反向归一化
- **不测范围**：真实上游 `opencode.ai` 的 500/503 瞬断不纳入自动化回归，保留 `mock` 为确定性信号

## Out of Scope

- `tools` 的命名空间限定（`namespace`）、`custom_tool_call` 与 `parallel_tool_calls` 的全量对齐
- `reasoning.effort` / `reasoning_content` 在 `Chat↔Responses` 间的有损/无损策略细化（保持现有直拷/合并逻辑）
- 网关二次勾选、权重与 `failover` 跨供应商重试语义的变更
- `CLIProxyAPI` 的 `gjson/sjson` 零分配实现移植与性能专项优化
- 存量 `config.json` 中三段式 ID 的批量迁移脚本（保留兼容，仅新增别名）

## Further Notes

- 本次修复已在线上通过 `curl` 对 `opencode.ai/zen/go/v1` 的真实上游验证：`Responses→Chat` 的 `input_text` 归一化后 `oc/chat/mimo-v2.5` 经 `/v1/responses` 非流式与流式均 `200`， `chat` 直连保持 `200`
- 短别名前提为同一供应商内模型 ID 全局唯一，当前 `oc` 的 `mimo`（仅 `chat`）与 `muse`（仅 `responses`）已满足；若未来出现同名跨渠道，需以三段式消歧
- 后续若需完全对齐 `CLIProxyAPI` 的工具链，建议另起 `ADR` 评估许可与实现范围
