Status: ready-for-agent

# pi 会话供应商侧扫盘替代客户端头注入

## Problem Statement

`pi-switch` 的“按对话分组”能力依赖 `pi` 扩展 `extensions/conversation-id-inject.ts` 在 `before_provider_headers` 注入 `x-conversation-id/name` 与 `x-opencode-session`。该链路与 `pi` 扩展分发强耦合：升级/卸载扩展即丢失对话分组，且开关 `settings.injectOpenCodeAttribution` 仅在扩展加载时读取，需重启 `pi`。`token 使用量/消费/推理 token/缓存命中率` 的解析已在代理侧完成（`StreamTee/SseUsageParser`），不依赖扩展；但对话维度的统计与请求明细分页完全依赖该注入。用户期望移除 `pi` 插件依赖，改为供应商侧（`pi-switch` 代理侧）提供会话归因，并提供可热生效的开关，`token` 统计不受影响。

## Solution

移除 `pi` 客户端注入，改为供应商侧扫盘发现 `pi` 会话：代理侧周期性扫描 `pi` 的 `sessions` 目录（复用 `cc-switch` 的根发现与 `JSONL` 树遍历规则），在统计查询时对未携带会话标识的近窗口请求按 `model + 时间窗口 ±2s`（辅以 `prompt_tokens` 校验）与扫盘结果虚拟 `JOIN`，生成对话分组；`requests.log` 保持追加式不可变。新增三档开关 `conversationSource: proxy | sessionScan | off`（默认 `sessionScan`），热生效；旧字段 `injectOpenCodeAttribution` 在首次保存时迁移并删除。

## User Stories

1. 作为 `pi` 用户，我想在不安装 `pi` 扩展的情况下仍能在统计页按对话查看 `token 使用量/消费`，以便延续按会话的成本分析。
2. 作为 `pi` 用户，我想在统计页看到每个对话的标题来自首条 `user` 消息或显式会话名，以便快速识别对话。
3. 作为 `pi` 用户，我想在请求明细中按对话筛选与分页浏览，以便定位某次对话的全部请求。
4. 作为 `pi` 用户，我想通过开关在“代理透传头兼容旧客户端 / 扫盘发现 / 完全不分组”三档间切换，以便按需控制会话分组行为。
5. 作为 `pi` 用户，我想切换开关后无需重启 `pi` 或代理即可生效，以便快速验证效果。
6. 作为 `pi` 用户，我想在 `off` 档时统计页隐藏对话 Tab，仅看全局与按 `provider/model` 聚合，以获得极简视图。
7. 作为 `pi` 用户，我想在 `proxy` 档时旧客户端携带的 `x-conversation-id` 仍被透传并用于分组，以便升级过渡期平滑。
8. 作为 `pi` 用户，我想在 `sessionScan` 档时未携带会话标识的请求能自动归入对应对话，而不是全部落入未标记，以便保持分组连续性。
9. 作为 `pi` 用户，我想让未标记请求仅在明确无对应会话时才归入 `unlabeled`，以避免误分组。
10. 作为 `pi` 用户，我想让 `--no-session` 与后台 `Magic Context` 这类内存会话不被计入分组，以免产生幽灵对话。
11. 作为 `pi-switch` 管理员，我想让旧配置中的 `injectOpenCodeAttribution` 自动迁移到新三档开关并被清理，以免配置残留。
12. 作为 `pi-switch` 管理员，我想让 `pi` 内仍可使用 `/piswitch` 斜杠命令，即使已移除会话注入扩展，以便不丢失交互入口。
13. 作为 `pi-switch` 管理员，我想在 `npm` 更新后 `pi` 侧自动感知扩展清单变化（仅移除注入项），而无需手动清理。
14. 作为开发者，我想让对话标题截断与清洗规则与现有 `webui` 展示一致（`60` 字符、控字符→空格），以保持视觉一致。
15. 作为开发者，我想让扫盘失败（目录不可读、相对路径需项目上下文等）仅 `warn` 并返回空，不阻断代理与统计主流程。
16. 作为运维，我想让扫盘轮询间隔固定为 `3s` 全量扫描，兼顾实时性与实现简洁。

## Implementation Decisions

- **移除范围**：仅删除 `conversation-id-inject` 注入扩展及其在 `pi` 扩展清单中的注册；保留斜杠命令扩展，包仍作为 `pi` 扩展分发，`pi` 内 `/piswitch` 可用。
- **供应商侧发现**：新增扫盘模块，根发现优先级 `PI_CODING_AGENT_SESSION_DIR` 环境变量 > `pi` 原生配置 `sessionDir` > 默认 `~/.pi/agent/sessions`；支持 `Flat` 与 `ProjectDirectories` 两布局；仅接受 `*.jsonl` 且 `≤128MB` / `≤500k` 条目，首行必须为 `type=session` 的合法头，否则跳过。
- **会话树遍历**：`version≥2` 时按 `id/parentId` 构树并从 `latest_id` 回溯活跃分支，`version<2` 按 `legacy-{index}` 线性链兼容；校验 `tree id` 字符集与长度，丢弃死分支与孤儿。
- **摘要提取**：显式 `session_info.name` 优先，其次首条 `user` 消息截断，再次 `cwd basename`；`last_active_at` 取所有消息 `timestamp` 最大值，用于排序与虚拟分组时间窗口。
- **关联策略**：`requests.log` 不回填；统计查询时对 `conversationId == None` 且落在近 `5min` 窗口的请求，按 `model + 时间窗口 ±2s` 与扫盘结果 `JOIN`，辅以 `prompt_tokens` 二阶校验去歧；`proxy` 档直接透传请求头中的会话标识，`off` 档跳过关联。
- **开关与迁移**：`settings` 新增枚举 `conversationSource` 三档，默认 `sessionScan`；旧布尔字段在首次 `save_config` 时映射并删除（`true→proxy/false→off`），配置版本号升至 `2`；轮询间隔为常量 `3s`，首版不暴露为配置。
- **过滤语义**：未落盘的内存会话（如后台任务）在扫盘中天然不可见，不产生分组，复刻原扩展对 `Magic Context` 的过滤效果；相对路径配置返回 `RequiresProjectContext` 并 `warn`。
- **展示**：`sessionScan/proxy` 档按对话聚合与分页，`off` 档隐藏对话 Tab；标题清洗复用现有 `60` 字符截断与控字符处理。

## Testing Decisions

- **测试理念**：仅测外部可观察行为，不测实现细节；以真实 `JSONL` 文件为输入断言解析与分组结果。
- **缝隙一（最高缝）—— 扫盘模块单测**：在隔离临时目录构造 `sessions` 布局（`Flat` 与 `ProjectDirectories`），覆盖根发现、布局分支、文件校验、头解析、活跃分支回溯、摘要截断与超限拒绝；复刻 `cc-switch` 的 `latest_leaf_defines_active_branch / duplicate_entry_ids / cycles` 用例为先验。
- **缝隙二（次高缝）—— 代理虚拟分组集成测**：在 `tokio` 环境下构造含 `requests.log` 窗口与扫盘 `Map` 的输入，断言 `sessionScan` 档的 `model+时间` 关联与 `off/proxy` 档的透传/隐藏行为，不依赖真实代理端口。
- **缝隙三（外层缝）—— `webui` 设置面板交互测**：断言三档切换互斥、`off` 时对话 Tab 隐藏、配置往返不丢字段；`token/cost` 解析不在本次缝隙内，已有覆盖且不受本改动影响。

## Out of Scope

- `token 使用量/消费/推理 token/缓存命中率` 的解析与计费口径变更（已在代理侧完成，不动）。
- 代理侧伪造会话 `UUID` 与 `Set-Cookie` 粘滞等有状态方案。
- `requests.log` 的回填或历史重写；已落盘历史保持不变。
- `pi` 核心 `getSessionHeaders` 的上游补丁或 `providerPrefix` 伪装。
- 增量扫盘、`inotify` 监听、扫盘间隔可配置化。
- 非 `pi` 客户端的会话分组（`cc-switch` 的 `Claude/Codex/Gemini` 扫盘不在本期）。

## Further Notes

- 本方案与 `ADR-0002` 冲突处以 `off` 档保留原语，`sessionScan` 为显式可开关的启发式发明，需在文档中明确近实时（`3s`）与亚秒并发错分的边界。
- 旧配置迁移为一次性显式操作，回滚旧版本将丢失新开关，需重新配置；首次迁移后 `injectOpenCodeAttribution` 不再出现于 `config.json`。
- 复用的 `cc-switch` 规则（根发现、布局、校验、树遍历）以其 `pi.rs` 为准，后续若 `pi` 会话格式演进需同步跟进。
