# 0011 pi-switch 核心领域的 Canonical Facts 与 Boundary Contract

Status: accepted

## Context

pi-switch 同时管理 `config.json`、`models.json`、SQLite request rows、pi session JSONL、WebUI draft 和 daemon 进程。历史缺陷反复来自同一事实在多个层级被重新解释：Gateway preview 与 publish 使用不同 normalization，Responses 由 handler 各自决定转换和 header，Stats 查询触发 migration 写入，WebUI 同时维护 JSON/structured/selection 多份状态，PID 文件被当成健康状态。

这些问题不能继续通过局部 fallback 修补。系统需要明确每个领域事实的唯一来源、派生视图、空值语义和边界验证，使后续 IMP-02～IMP-05 能以同一 contract 实施。

## Decision

1. `config.json` 的 Supplier/Channel/Model/exposed model 是供应商事实来源；Gateway 是显式发布的派生视图，不因 Supplier 或 settings 变更自动写入。
2. Gateway 由后端 `CanonicalGatewayPlan` 生成；Preview、Diff、Validation、Publish 必须消费同一 plan。第三方 provider 原样保留，冲突时零写入，连续发布收敛到 `pending_count=0`；已发布 Gateway 的手工 model metadata（`name`、`headers`、`compat`、`extra`）由 current 合并回 proposal，显式 draft 优先，不回写 Supplier config。
3. SQLite request rows 保存 immutable request facts；conversation attribution 是独立派生视图；Stats GET 不执行 migration 写入。
4. Responses 路由以 provider 声明的 `api` 和 `responsesMode` 为能力 contract，不运行时探测、不静默降级、不引入 failover。所有 outbound request 路径共享 builder；所有 token clamp 路径共享唯一实现。
5. WebUI 使用一个 canonical draft；Config JSON 保持直接可编辑，Format 在左下，错误只标记真实位置。
6. daemon 的 managed identity 与 TCP health 共同决定运行态；PID 文件只是定位信息；本期每类服务只支持一个受管理实例。
7. 缺失、`null`、空数组和空字符串不互换；默认值只在唯一 boundary 产生；无法按 contract 解释的输入在发送 upstream 或写入文件前失败。

## Consequences

### Positive

- Preview 与 Publish 不会因重复 normalization 产生永久 pending。
- 请求首次、stream、retry 的 URL/header/affinity policy 可以由同一 builder 覆盖。
- clamp、Responses streaming 和 usage 的回归测试能引用同一条明确 contract。
- Stats 的事实记录不会被事后归属或展示格式重写。
- 前端和 daemon 的真实验收边界可由固定 viewport、真实 session 目录和真实进程测试覆盖。

### Costs

- 旧配置需要在 migration boundary 明确处理，不能在普通业务路径隐式兼容。
- 部分已有 handler、前端状态和测试必须改为消费 typed canonical object。
- 真实验收需要可运行的容器、upstream 或浏览器环境，单元测试不能替代这些检查。

## Rejected alternatives

- 在每个 handler 中继续增加 fallback：会保持多套事实解释，不能保证路径一致。
- 通过扩大 session 时间窗口解决会话歧义：会把错误归属伪装成成功匹配。
- 让 WebUI 继续重建 Gateway provider：会重新引入前后端 normalization 分叉。
- 让 PID 文件存在直接表示 daemon running：无法区分 stale PID、端口冲突和进程不健康。

## Verification obligations

- 具体条款和回归矩阵见 `docs/system-contract.md`。
- IMP-02～IMP-05 每项独立完成 targeted tests、静态检查、真实功能验证和单独提交。
