# 规格文档导航

集中收录原 `.scratch/<主题>/spec.md` 和原 `docs/plans/mygo/SPEC-*.md`，当前共 **35 份规格**，文件平铺存放。

这些规格来自不同迭代阶段，不等同于当前待办或实现结论。原文中的 `ready-for-agent` 等状态未经重新核验；阅读时应结合当前实现和 [ADR 决策记录](../docs/adr/)，不要将历史规格直接当作执行指令。

## 分类导航

- [架构与迁移](#architecture)（5 份）
- [供应商、配置与 Gateway](#providers-config-gateway)（10 份）
- [协议、路由与请求处理](#protocol-routing)（4 份）
- [模型目录与元数据](#model-metadata)（3 份）
- [统计、消费与会话](#analytics-sessions)（11 份）
- [测试与缺陷记录](#tests-defects)（2 份）

<a id="architecture"></a>

## 架构与迁移

| 规格 | 关注内容 |
|---|---|
| [Architecture V2 模块化单体重构](./architecture-v2.md) | 模块分层、业务用例、依赖方向与兼容边界。 |
| [MyGo Native GUI 全面迁移](./mygo-native-gui-001.md) | 原生 GUI 的目标形态、共享业务服务与迁移验收要求。 |
| [移除 Rust，收敛为 Go 单核](./remove-rust-core.md) | 移除 Rust 工具链、原生产物与双核维护链路。 |
| [Go 重写增量与前后端对接](./rewrite-go-increment.md) | API 合约、界面状态同步、构建嵌入及端到端对接。 |
| [Go 重写总体规格](./rewrite-go.md) | 从 Rust 与 TypeScript 技术栈迁移至 Go 的总体设计。 |

<a id="providers-config-gateway"></a>

## 供应商、配置与 Gateway

| 规格 | 关注内容 |
|---|---|
| [配置 JSON 编辑与模型格式对齐](./config-json-editable.md) | 直接编辑模型 JSON，并与 Pi 的模型条目格式对齐。 |
| [固定双 provider 的网关聚合](./fixed-dual-gateway-providers.md) | 按 Responses 与 Chat API 合约聚合网关模型。 |
| [按渠道组织 provider 与裸模型 ID](./gateway-per-channel-bare-id.md) | 以渠道划分 provider，区分模型身份与内部路由。 |
| [Gateway 独立管理界面](./gateway-sep-webui.md) | 拆开本地配置保存与网关发布，在网关面板预览并显式应用。 |
| [JSON 错误标记与 Responses 测试](./inline-json-highlight.md) | 配置编辑器中的错误定位、标记与 Responses 测试支持。 |
| [渠道级 API 隔离](./per-channel-api.md) | 按渠道声明 API 与 Responses 模式，选择请求转换路径。 |
| [渠道模型分区与网关选择](./supplier-channel-models.md) | 按渠道管理模型池、暴露集合及网关发布子集。 |
| [供应商余量查询](./supplier-credits-panel.md) | 查询上游余量并归一化展示，包含刷新与错误反馈。 |
| [供应商与 Gateway 独立、显式发布](./supplier-gateway-isolation.md) | 配置事实与网关派生视图解耦，约束自动写入及错误传播。 |
| [Gateway 发布前编辑](./webui-gateway-preedit.md) | 写入模型文件前预览差异、编辑 JSON 并保留手工字段。 |

<a id="protocol-routing"></a>

## 协议、路由与请求处理

| 规格 | 关注内容 |
|---|---|
| [Chat/Responses 归一化与短模型 ID](./chat-responses-normalization-short-id.md) | 双向转换消息内容类型，并讨论短模型 ID 的渠道定位与歧义。 |
| [Responses 上下文溢出处理](./fix-400-context-overflow.md) | 通过静态截断与动态 Token 夹逼处理上下文溢出请求。 |
| [移除故障转移链](./remove-failover-chain.md) | 清理全局故障转移与重试策略，保留后续熔断设计边界。 |
| [Responses 透传与协议转换](./responses-provider-passthrough.md) | 依据上游 API 能力选择 Responses 透传或协议转换。 |

<a id="model-metadata"></a>

## 模型目录与元数据

| 规格 | 关注内容 |
|---|---|
| [模型元数据自动注入与覆盖规则](./auto-enrich.md) | 处理目录查找去歧义、默认值覆盖与自动补齐链路。 |
| [Gateway 模型元数据补齐](./gateway-catalog-enrich.md) | 在网关预览与发布时使用模型目录补齐缺失字段。 |
| [models.dev 模型目录更新](./models-dev-enrich.md) | 明确元数据来源、模型更新触发、字段补齐与缓存策略。 |

<a id="analytics-sessions"></a>

## 统计、消费与会话

| 规格 | 关注内容 |
|---|---|
| [客户端注入对话标识](./conversation-id-inject.md) | 通过 Pi 扩展向请求注入会话标识，支撑对话分组。 |
| [每百万 Token 的成本换算](./cost-per-million.md) | 修正价格单位换算，并保留历史请求的原始费用事实。 |
| [消费统计与刷新](./cost-stats.md) | 记录请求时的费用，按总量、对话与请求展示并定时刷新。 |
| [供应商侧扫描 Pi 会话](./pi-session-supplier-scan.md) | 由代理侧扫描会话文件进行归因，减少客户端注入依赖。 |
| [单次请求 Token 明细](./request-token-detail.md) | 展示每次请求的 Token 使用细项与缓存信息。 |
| [统计页对话名称与布局](./stats-conversation-name.md) | 展示可读会话名称，调整请求明细与对话聚合的布局。 |
| [会话名称的首条消息回退](./stats-conversation-title.md) | 无显式名称时使用首条用户消息作为显示名称。 |
| [历史请求日志兼容](./stats-legacy-compat.md) | 导入旧请求日志并保持新请求双写，延续历史统计。 |
| [请求明细分页](./stats-request-pagination.md) | 在统计窗口内浏览历史请求，支持页码与每页条数切换。 |
| [推理 Token 与统计窗口](./token-stats-reasoning-window.md) | 区分推理 Token 子集，并按日期或时间区间聚合统计。 |
| [Token 总量、对话用量与缓存命中率](./token-usage-stats.md) | 从响应中解析用量，记录请求事实并提供汇总与对话统计。 |

<a id="tests-defects"></a>

## 测试与缺陷记录

| 规格 | 关注内容 |
|---|---|
| [历史未决缺陷台账](./known-gaps.md) | 记录缺陷证据、处理情况与仍需决策的问题。 |
| [历史 WebUI 端到端测试方案](./webui-e2e.md) | 浏览器界面下供应商、模型、网关与请求统计的端到端验证。 |

## 相关材料

- [ADR 决策记录](../docs/adr/)：查看设计决策及其背景。
- [文档归档](../archive/README.md)：查阅计划、票据、研究笔记与测试报告。
