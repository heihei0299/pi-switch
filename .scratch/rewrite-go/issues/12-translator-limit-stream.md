# 12: 协议转换与限流流式（translator 复用 + responsesMode + StreamTee）

**What to build:** 在 11 的网关与 failover 上叠加协议与限流完整能力：直接复用 `CLIProxyAPI/internal/translator` + `sdk/translator`（裁剪 Gemini，仅保留 `openai-completions`/`openai-responses`/`anthropic-messages` 三路径），按 `ProviderProfile.api × responsesMode`（`auto/passthrough/convert`，`auto` 下 `openai-responses→passthrough`、`openai-completions→convert`，`passthrough↔openai-responses`/`convert↔openai-completions` 不兼容保存时阻止）分发，`POST /v1/responses` 与 `POST /v1/messages` 端到端互转（含流式 `ChatSseToResponses`/`responses_to_chat`），`contextWindow` 限流按 spec 完整公式（`est=ceil(jsonLen/4)` 含 `tools`，`available=contextWindow-est-4096`，`clamped=max(16,available)` 与 `maxTokens` 取最小，覆写 `max_output_tokens/max_tokens/max_completion_tokens`），SSE/WebSocket 经 `StreamTee` 分流到客户端与 `SseUsageParser`，`x-conversation-name` 的非 Latin1 字符 percent-encode 对称解码后在统计中可读。复用原型 `prototype/go-skeleton-gin-sqlite:clampMaxTokens/StubTranslator` 提升到 `internal/proxy/limit+convert`。

**Blocked by:** 11 多供应商与网关发布

**Status:** ready-for-agent

- [ ] `api=openai-responses` 的 `Supplier` 收到 `POST /v1/responses` 时透传（SSE 事件原样），否则走转换；`api=anthropic-messages` 的 `POST /v1/messages` 与 `openai` 互转后上游可用
- [ ] `responsesMode` 不兼容组合在 `PUT /api/profiles` 保存时返回 400，`auto` 语义与 spec 一致
- [ ] `max_output_tokens`/`max_tokens`/`max_completion_tokens` 按 `ModelEntry.contextWindow/maxTokens` 与输入估算自动重写，超窗 400 溢出不再出现（含 `reasoning.encrypted_content` 低估边界，`est` 已含序列化长度）
- [ ] 流式：`POST /v1/chat/completions`（`stream=true`）与 `POST /v1/responses` 流式经 `StreamTee` 完整转发且 `prompt/completion/cached/reasoning` 四部分用量被解析落盘；`GET /api/stats` 的 `cached` 与 `reasoning` 列正确
