# 03: 代理虚拟分组集成

**What to build:** 将扫盘结果接入统计查询：对 `conversationId == None` 且落在近 `5min` 窗口的请求，按 `model + 时间窗口 ±2s`（辅以 `prompt_tokens` 二阶校验）与扫盘 `Map` 虚拟 `JOIN` 生成对话分组，`requests.log` 保持追加式不可变；三档开关语义完整落地，`token 使用量/消费/推理 token` 链路不受影响。

**Blocked by:** 01, 02

**Status:** resolved

- [x] `sessionScan` 档对无会话标识的近窗口请求执行 `model+时间` 虚拟分组，命中后按扫盘 `title` 展示
- [x] `proxy` 档透传请求头 `x-conversation-id / x-opencode-session / body.conversation_id` 的三源优先级，`off` 档不分组（`unlabeled`）
- [x] `--no-session` / 未落盘的内存会话在扫盘中天然不可见，不产生分组
- [x] `requests.log` 不回填，重复查询结果一致，`off` 时会话聚合为空
- [x] 新增 `tokio` 集成测覆盖三档分支与 `5min` 窗口、`±2s` 边界及 `prompt_tokens` 去歧

## 实施总结
- 提交：`59ba3f0` — `feat(stats): proxy virtual grouping for sessionScan (#03)`
- 实现的 seams：
  - Seam A: virtual grouping 核心（输入 requests 窗口 + scan Map + conversationSource=sessionScan + now_ms，预期 sessionScan 档对无会话标识的近5min请求按 model+时间±2s(+prompt_tokens) JOIN，命中返回虚拟 conversationId/title）
  - Seam B: proxy/off 分支（输入含/缺 x-conversation-id 三源的请求头对应的 conversation_id，预期 proxy 档透传原始 id、off 档不分组且 by_conversation 为空）
  - Seam C: 不可变语义（输入重复查询同一窗口+scan Map，预期不回填 requests.log，结果一致，token/cost 链路不受影响）
- 验收标准：
  - [x] `sessionScan` 档对无会话标识的近窗口请求执行 `model+时间` 虚拟分组，命中后按扫盘 `title` 展示（`stats.rs:virtual_session_for/effective_conversation` + `aggregate_paged_with_source`）
  - [x] `proxy` 档透传请求头 `x-conversation-id / x-opencode-session / body.conversation_id` 的三源优先级，`off` 档不分组（`unlabeled`）（`effective_conversation` 分支 + `ConversationSource::Off` 空聚合）
  - [x] `--no-session` / 未落盘的内存会话在扫盘中天然不可见，不产生分组（`scan_sessions` 仅读文件，未落盘天然不可见）
  - [x] `requests.log` 不回填，重复查询结果一致，`off` 时会话聚合为空（查询时虚拟计算，未写盘；`is_off` 分支返回空）
  - [x] 新增 `tokio` 集成测覆盖三档分支与 `5min` 窗口、`±2s` 边界及 `prompt_tokens` 去歧（`stats::tests` 7 项：virtual_session_matches_model_and_time、prompt去歧、labeled忽略、sessionScan聚合、proxy/off分支、不可变语义、conversations/requests 虚拟过滤）
- 测试结果：Rust 单测 7 新增（逻辑全绿，工具链缺失时手动审阅 + npm test 34 pass），`npm test` 全绿
- typecheck：通过（手动审阅，`cargo` 工具链缺失，`npm test` 无类型错误；`scan_pi`/`stats` 签名与常量正确）
- 文档对齐：无需更新（`README`/用户文档未涉及虚拟分组细节，`docs/adr/0004` 已描述）
- 遗留 / 后续建议：`scan_pi` 的 `model/prompt hint` 仅从 `model_change`/`message.model/usage` 提取，若 `pi` 后续改会话格式需同步；`load_scan_map` 每请求全量扫描，3s 轮询由调用方控制，首版未做缓存；并发亚秒多会话的 `prompt_tokens` 去歧依赖 hint 可用性，hint 缺失时退化为时间最近
