# ADR-0004: pi 会话由供应商侧扫盘发现替代客户端头注入

日期: 2026-09-01
状态: 已接受
关联: ADR-0002（对话边界由客户端标识决定）

## 背景

`pi` 的会话 `id` 仅活在 `pi` 进程内存 `sessionManager`，此前由 `pi-switch` 的 `pi` 扩展 `extensions/conversation-id-inject.ts` 在 `before_provider_headers` 注入 `x-conversation-id/name` 与 `x-opencode-session`，`proxy` 仅读取（`proxy.rs:conversation_id_of/name_of`）并写入 `requests.log`。该链路强耦合 `pi` 扩展分发（`package.json#pi.extensions`），`token` 统计以外的“按会话分组”能力与 `pi` 侧发布强绑定；升级/卸载扩展即丢失分组，且 `settings.injectOpenCodeAttribution` 开关仅在扩展加载时读取，需重启 `pi`。

`cc-switch` 对 `pi` 采用供应商侧扫盘（`src-tauri/src/session_manager/providers/pi.rs`）：按 `PI_CODING_AGENT_SESSION_DIR` > `pi` 配置 `sessionDir` > `~/.pi/agent/sessions` 解析 `Flat / ProjectDirectories` 布局，直接解析 `JSONL` 会话树，与代理日志无关。

## 决定

1. **移除客户端注入**：删除 `extensions/conversation-id-inject.ts` 及其在 `package.json#pi.extensions` 的注册，包保留 `extensions/index.ts`（`/piswitch` 斜杠命令）以保持分发兼容。
2. **供应商侧扫盘**：在 `pi-switch` Rust 侧新增 `scan_pi` 模块，复用 `cc-switch` 的根发现、布局、校验与树遍历规则（`MAX_SESSION_BYTES 128MB / MAX_TREE_ENTRIES 500k / TITLE_MAX_LEN 60`），`3s` 全量轮询 `sessions` 目录，`parse_session` 提取 `{id, title, last_active_at}`。
3. **会话关联为查询时虚拟分组**：`requests.log` 维持追加式不可变，不回填；`stats` 查询时对 `conversationId == None` 且落在近 `5min` 窗口的请求，按 `model + 时间窗口 ±2s`（辅以 `prompt_tokens` 二阶校验）与扫盘结果 `JOIN`，生成虚拟 `conversationId`。该行为由三档开关控制。
4. **三档开关**：`settings.conversationSource: "proxy" | "sessionScan" | "off"`（默认 `sessionScan`），`proxy` 档保留透传头兼容旧客户端，`off` 档不分组并在 `webui/TUI` 隐藏会话 Tab。旧字段 `settings.injectOpenCodeAttribution` 在首次 `save_config` 时显式迁移并删除，`config version` 升至 `2`。
5. **不可见会话过滤**：`--no-session` / `MAGIC_CONTEXT_PI_SUBAGENT` 这类未落盘的内存会话在扫盘中天然不可见，不计入分组（与 `cc-switch` 的 `RequiresProjectContext` 行为一致）。

## 后果

- 正面：`pi-switch` 脱离 `pi` 扩展分发，会话分组能力收敛到供应商侧，开关热生效（`load_config` 每请求读取），`token/cost` 链路不受影响。
- 负面：与 `ADR-0002` 的“代理不自行发明对话边界”冲突——`sessionScan` 档是显式的启发式发明，需以 `off` 档保留原语；近实时（`3s`）而非实时，亚秒并发会话可能错分。
- 迁移：旧 `config.json` 含 `injectOpenCodeAttribution` 的用户在升级后首次保存时自动迁移，无感；回滚旧版本会丢失新开关（需重新配置）。

## 备选

- 伪装 `providerPrefix=opencode` 以复用 `pi` 核心 `getSessionHeaders`：改动更小但依赖 `pi` 核心判定且无法提供三档语义，否决。
- 代理侧伪造 `UUID` 并 `Set-Cookie` 粘滞：引入有状态，需持久化与过期管理，YAGNI，否决。
- 离线 `requests.log` 回填：破坏追加式不可变语义，回滚不可逆，否决。
