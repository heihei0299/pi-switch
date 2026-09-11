# pi-switch Context

pi-switch 是 pi 客户端（Perplexity 的模型客户端）的轻量 profile 切换器：管理多个 provider profile，并提供一个本地代理，把客户端请求路由到当前 profile 的上游服务，支持 OpenAI/Anthropic 格式互转。

## Language

**Token 使用量（Token Usage）**：
单次请求消耗的 token 数，分四部分：输入（prompt）、输出（completion）、命中缓存输入（cached）、推理（reasoning）。由上游响应中的 usage 数据解析得出；取不到时该请求记为 unknown。
_Avoid_: 用量、消耗、费用

**消费（Cost）**：
由 token 使用量乘以模型单价折算的估算费用。单价按**每 1M tokens** 计（行业惯例，如 `input: 2.0` 表示 $2 / 1M tokens），来自 profile 模型配置（input/output/cacheRead/cacheWrite，可分级），在请求完成时定格并写入请求日志；模型未配置单价时该请求的消费记为 unknown。
_Avoid_: 费用、花费、金额、账单
**推理 token（Reasoning Tokens）**：
输出 token 中用于模型推理思考的部分，取自 `completion_tokens_details.reasoning_tokens`（Chat Completions / DeepSeek）或 `output_tokens_details.reasoning_tokens`（Responses）；是输出 token 的子集，总数不重复累加。上游不报告的（如 Anthropic）记 0。
_Avoid_: 思考 token、思维链 token

**统计窗口（Stats Window）**：
统计页的时间过滤范围。预设四种：当天（本地时区自然日）、24 小时以内（滚动）、7 天以内（滚动）、自定义日期区间（`[起日 0 点, 止日 24 点)`）。窗口作用于整个统计页的所有聚合。
_Avoid_: 时间段、筛选、时间范围

**缓存命中率（Cache Hit Rate）**：
命中缓存的输入 token ÷ 总输入 token，以百分比显示。分母只含输入 token，不含输出。
_Avoid_: 缓存率百分比、token 节省率

**对话（Conversation）**：
由客户端在请求中携带的会话标识（`x-conversation-id` 请求头、`x-opencode-session` 请求头（pi 核心 v0.84.1 在 provider 为 opencode / opencode-go 或 baseUrl 为 opencode.ai 时自动注入，值=当前会话 id，并同时注入 `x-opencode-client: "pi"`；opencode 客户端亦发送）或 body 的 `conversation_id` 字段）标识的一组请求，按标识聚合成一次对话的 token 统计。
_Avoid_: 会话、thread、session
**Responses 透传模式（Responses Passthrough Mode）**：provider 原生支持 Responses API 时，客户端的 Responses 请求、响应及 streaming 事件保持原有语义传递；不具备原生支持的 provider 使用 Responses 与 Chat Completions 之间的转换。

**未标记（Unlabeled）**：
没有携带会话标识的请求，统计时归入名为 `unlabeled` 的单一组。
_Avoid_: unknown、无会话

**请求日志（Request Log）**：
`requests.log`，每行一个 JSON 的追加式文件，记录每次代理请求的元数据（时间、成败、provider、model、延迟、token 使用量、消费、会话标识）。
_Avoid_: 日志文件、usage log

**请求明细（Request Details）**：
Stats 页中按时间倒序逐条展示请求日志记录的表格区块，分页浏览，可覆盖窗口内全部历史。
_Avoid_: 请求记录、明细表、recent requests

**模型目录（Model Catalog）**：
https://models.dev 提供的 provider→models 元数据目录，每次更新模型时作为权威元数据源；与上游 `/v1/models` 仅返回 id 列表不同，目录提供 `cost/limit/reasoning/modalities` 等可用于 enrich 的完整参数。
_Avoid_: 模型源、模型网站

**模型元数据（Model Metadata）**：
目录中单条模型的字段集合，包含 `cost`（input/output/cacheRead，按 $/1M）、`limit.context/output`（映射为 contextWindow/maxTokens）、`reasoning`、`modalities.input`（映射为 input）与 `name`；enrich 时按分字段策略与本地 ModelEntry 合并。
_Avoid_: 模型参数、模型信息

**供应商（Supplier）**：
对应一个 `ProviderProfile`（`config.json: profiles[name]`），容纳多条渠道（Upstream/Channel），每条渠道独立拥有 `api`、模型池 `models[]` 与已暴露集 `exposedModels[]`，为模型与凭证的唯一事实来源。
_Avoid_: profile、provider、供应商配置

**渠道（Channel）**：
上游（Upstream）的同义词，指供应商下的单条上游连接，`name` 为渠道主键（同一供应商内唯一必填），含 `api/baseUrl/apiKey/headers/weight`。每条渠道独立拉取与独立暴露；多个渠道可声明同一模型 id，但网关发布时所有 exposed model 的裸 id 必须全局唯一。
_Avoid_: 通道、节点、endpoint、渠道配置

**上游（Upstream）**：
见渠道（Channel）；持久化上每条渠道含 `api/baseUrl/apiKey/headers/weight/name` 与分区字段 `models[]/exposedModels[]`。
_Avoid_: endpoint、upstream 配置、节点

**API 能力（API Capability）**：
`internal/protocol` 对每个 api 标识的三项判定：`IsKnown`（是否为受支持标识）、`CanProxy`（translator 能否路由它）、`CanGateway`（能否发布到网关）。同一份来源同时决定预设列表的过滤、写入口的能力判定与 WebUI 可选项；"已知但当前不可代理"（如 `google-generative-ai`）是合法且必须被保留的状态，不是配置错误。
_Avoid_: 支持列表、白名单、能力表副本

**写入门（Write Door）**：
配置进入磁盘的入口及其校验强度：整文件门（`PUT /api/config`）、Profile CRUD（`POST`/`PUT /api/profiles`、CLI `provider add`）、duplicate（`…/:name/duplicate`、CLI `provider duplicate`）、advisory（`GET /api/config/validate`，只报不写）。能力判定由各自入口共用 `config.ValidateResolvedCapability`；各门之间**有意保留**的强度差异与其理由见 `docs/system-contract.md` §2.2，不得为了"统一"而顺手放宽或收紧。
_Avoid_: 校验器、validator、统一校验层

**网关（Gateway）**：
写入 `models.json: providers` 的面向客户端 provider 视图，由当前 Supplier/Channel 的 exposed model 按 API contract 聚合为 `pi-switch-res`（Responses）与 `pi-switch-chat`（Chat）；provider 内的模型 id 为裸 `modelId`，设置只保留本地 proxy host/port 等运行参数。
_Avoid_: gateway provider、pi gateway、网关配置

**网关发布（Gateway Publish）**：
将当前 gateway provider 草稿显式写入网关文件的唯一路径（`PUT /models/gateway` / GatewayPanel「应用到 Pi」）；草稿只维护有 exposed model 的 `pi-switch-res` 与 `pi-switch-chat`，每个 provider 的 API contract 固定，模型 id 保持裸名。发布外任何供应商变更均不自动写网关。
_Avoid_: 同步、自动同步、apply

## System Contract
**Canonical facts and boundary contract**：各领域唯一事实来源、缺失/`null`/`[]` 语义、Gateway/Responses/Stats/WebUI/daemon 不变量和 IMP-01～IMP-05 追踪矩阵见 `docs/system-contract.md`。实现与测试必须引用该 contract；不能用未定义的 fallback、自动探测或重复 projection 替代明确边界。
**ADR draft**：对应决策草稿暂存于 `.scratch/bug-history/adr-draft-canonical-system-contract.md`；未经显式确认不得写入 `docs/adr/`。
