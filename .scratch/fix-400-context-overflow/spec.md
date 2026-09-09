# Spec: 修复 Responses 上下文溢出 400（对齐 CLIProxyAPI 静态截断 + 保守动态夹逼）

Status: ready-for-agent
Version: 2.0
Author: pi-switch maintainers
Date: 2026-09-07
Scope: 代理转发 `clamp` / `limit` / `Responses` 透传（`pi-switch` 侧），不含 `pi` 侧 `compaction`

## Problem Statement

用户在长会话中（`muse-spark-1.2-contributor`，`contextWindow=1_048_576`，`maxTokens=943_718`，`xhigh` 推理，携带 `reasoning.encrypted_content` base64 密文）二次进入 `plan 模式` 后，`pi → pi-switch → opencode-go (https://opencode.ai/zen/go/v1/responses)` 的请求被上游以 `400 invalid_request_error: input_tokens + max_output_tokens ≤ 1048576` 直拒，`pi` 侧仅见 `Error from provider (Console Go): Upstream request failed`，在会话 `01a076fa-2426-7390-a782-71b1f08fe4ff` 中 `652/654/656/660`（`14:51-15:01`）连续四次 `继续` 必现。

`pi-switch` 现有 `internal/limit.ClampMaxTokens` 以 `est=ceil(rawJSONLen/4)`、`available=window-est-4096` 估算输入，对 `encrypted_content`（≈2.5 chars/token）低估 60-70k，`available` 高估后 `clamped=min(available,maxTokens)=~640k`，`414k(prompt+cacheRead)+640k≈1_054k>1_048_433` 仍溢出。故障前成功请求的 `cacheRead=413_553, total≈414k` 已逼近阈值，`switch to per-conversation breaker` 后单候选直通不再有跨供应商 failover 掩盖，400 直接暴露。

参考 `router-for-me/CLIProxyAPI`（`capGeminiMaxOutputTokens`/`ensureModelMaxTokens`/`normalizeClaudeBudget`）：仅当 `requested > 静态上限(OutputTokenLimit/MaxCompletionTokens)` 时截断，未携带 `max_*` 则不注入，不做 `window-est` 动态夹逼，`opencode` 侧无专项补偿，靠上游自校验。

## Solution

对齐 `CLIProxyAPI` 的“静态上限截断优先”，叠加 `pi-switch` 的“保守动态可用量”兜底，形成“静态截断 + 保守估算 + 自纠重试 + 可观测”闭环：估算对 `reasoning.encrypted_content` 单独按 `len/2.5`、其余按 `len/3`，`safety=max(8192, window/128)`，`available=window-est-safety`，`effective=max(available,16)∩maxTokens`，未携带 `max_*` 的短请求不注入；`est>window-16` 时直接 `max=16` 且告警；对 `400 invalid_request_error` 首包执行一次自纠重试（`max→16` 或剥离加密内容）；真实 `prompt_tokens` 回写校准下次估算；`clamp` 与 `400` 均落盘可复盘。

## User Stories

1. As a pi 用户，我在 `oc/muse-spark-1.2-contributor` 长会话（`cacheRead>400k` + `xhigh`）二次进入 `plan` 时，不再见 400，`max_output_tokens` 被自动压至可用量内（≈630k）仍成功返回
2. As a pi 用户，我在短会话（`input 5k`）时 `max_output_tokens` 不被过度压缩，仍接近模型 `EffectiveMaxTokens`
3. As a pi 用户，我在携带 `reasoning.encrypted_content` 的请求中，代理的估算能贴近服务端真实分词，不再低估 60k
4. As a pi 用户，我在 `input` 本身濒窗（`est>window-16`）时能收到 `max=16` 的短答而非 400 循环
5. As a pi 用户，我在偶发 400 后代理能当次自纠重试一次（`max=16` 或去加密内容），无需手动 `继续`
6. As a pi 用户，我想在 `clamp` 生效时看到 `info` 日志 `clamped 908720→761058 est 285k window 1048576`，判断是否濒窗
7. As a pi 用户，我想在 400 发生时代理在 `~/.pi-switch/400-dump/` 留下截断请求体与估算明细，供定位而非盲猜
8. As a 开发者，我想该逻辑仅改 `limit`/`server` 的估算与重试分支，不引入 `tiktoken` 重依赖
9. As a 开发者，我想对齐 `CLIProxyAPI` 的静态截断语义：未携带 `max_*` 不注入，已携带且 `≤ limit` 不重写
10. As a 运维，我想 `dev`/`upstream` 合并本 fix 时仅 `limit.go`/`server.go` 冲突，易合入

## Implementation Decisions

- **估算**：沿用 `internal/limit` 与 `internal/proxy/limit` 的 `ClampMaxTokens` 签名，内部拆分：`input` 中 `reasoning.encrypted_content` 段（如存在）按 `len/2.5` 计，其余（`tools`/`instructions`/剩余 `input`）按 `len/3`，`ceil` 求和；`internal/server` 的 `clampBody` 保持对 `max_tokens`/`max_output_tokens`/`max_completion_tokens` 三键的遍历，传入的 `rawLen` 改为区分段后的等效字符数。`safety = max(8192, window/128)` 动态缓冲。
- **截断**：`available = window - est - safety`，`effective = max(available, 16)` 再与 `ModelEntry.EffectiveMaxTokens` 取小；仅当 `requested != nil && *requested > effective` 时重写，且遵循 `CLIProxyAPI` 的“静态上限”对齐：`effective = min(effective, staticLimit)`。`est > window-16` 时 `effective=16` 并 `warn`。
- **未携带不注入**：对齐 `capGeminiMaxOutputTokens` / `ensureModelMaxTokens` 的“缺省不补”语义：`requested == nil` 的 `Responses` 请求不注入 `max_output_tokens`，由上游默认值决定，避免短请求被误压。
- **真值闭环**：进程内 `LRU<modelId, lastPromptTokens>` 缓存上次 200 的 `usage.prompt_tokens`，每次 200 回写；下次 `est = max(启发式, last*1.05)`（1.05 为服务端与本地差异缓冲），比本地分词器更准且零体积。
- **自纠重试**：仅对 `400` 且 `body` 含 `invalid_request_error` 的 `handleChatCompletions`/`handleStream` 首包执行一次：优先 `max_output_tokens=16` 重发，若仍 400 则剥离 `reasoning.encrypted_content` 重发；成功按 200 记账，失败透传原 400。`stream` 仅在首包/头未发出前可重试，不引入 `retry.go` 的全局 `cooldown`（已休眠，待 `per-conversation breaker`）。
- **可观测**：`clamp` 命中 `info(model, est, max_before, max_after, window)`；`400` 异步落盘 `~/.pi-switch/400-dump/<ts>-<model>.json`（含 `est`/`max_before`/`max_after`/`window`/`body_trunc`），保留最近 10 个滚动；管理侧只读 `GET /api/dumps` 暴露，不新增写接口。
- **兼容**：`contextWindow==0` 不 `clamp`；`max` 缺失按“未携带不注入”处理；`upstream/main` 仅字段映射的透传被本 fix 覆盖，合并不冲突；`AD R 0005` 的 `models.dev` 仍为 `contextWindow/maxTokens` 权威源，`cache` 24h TTL 不变。

## Testing Decisions

- 只测外部行为，不测内部遍历顺序；`retry.go` 保持休眠，仅测本 `clamp`/`自纠` 分支。
- **新增单测**（复用 `NewProxyRouter` + `httptest` 上游 Mock）：
  - `clamp_with_encrypted_content`：含 `encrypted_content` 长段的 `Responses` 体，断言 `max 908720→~761k` 且 200，验证 `len/2.5` 分支
  - `clamp_short_request_not_injected`：未携带 `max_*` 的短请求不被注入 `max_output_tokens`
  - `clamp_chat_len3`：`Chat` 路径按 `len/3` 截断
  - `retry_on_400_to_min`：首包 400 `invalid_request_error`，代理以 `max=16` 重试后 200
  - `est_past_window_warn`：`est>window-16` 时 `max=16` 且 `warn` 分支
- **存量回归**：`go test ./internal/...` 与 `NODE_ENV=test npm --prefix webui run test` 全过；`internal/limit` 与 `internal/proxy/limit` 各自单测保留；不引入 `tiktoken`，`cargo` 类比项不适用。

## Out of Scope

- 引入 `tiktoken` 或模型专属分词器
- `pi` 侧 `plan` 注入去重/精简（`pi-plan-mode` 扩展范畴）
- TUI 展示 `400-dump`
- 对 `input` 本身超窗的 `compaction`/截断历史（`pi` 侧 `compaction` 负责）
- `per-conversation breaker` 熔断（本 fix 仅保留 `retry.go` 休眠，不复用其 `cooldown`）

## Further Notes

- 本 fix 是对 `fix-400-context-overflow v1`（`xhigh` 二次 `plan` 溢出）与本次会话 `01a076fa` 四连 400 的合并收敛，参考 `router-for-me/CLIProxyAPI` 的 `capGeminiMaxOutputTokens`（`internal/runtime/executor/gemini_executor.go:918`）、`ensureModelMaxTokens`（`claude_executor_cloaking.go:2137`）、`normalizeClaudeBudget`（`thinking/provider/claude/apply.go:171`）的静态截断思路，差异点为 `pi-switch` 保留 `window-est` 动态兜底以应对 `opencode 1048433` 严格阈值。
- 二次 `plan` 为最长上下文点，`reasoning.encrypted_content` 为主因；`recentRequests` 与 `conversationId=null` 的展示为 `x-conversation-id` 缺头时的历史侧效应，与病因无关。
- 不新增 `ADR`：决策可低成本逆转（`revert` 单文件），无难逆转权衡；`CONTEXT.md` 的“支持 OpenAI/Anthropic 格式互转”与 `ADR 0004` 的 `Responses` 透传/转换语义保持不变。
