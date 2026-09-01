# 10: 单供应商代理闭环（passthrough + SQLite 写入 + 统计查询）

**What to build:** 在 09 基座上闭合首个端到端代理链路：单 `Supplier`（`test-provider`）单 `Upstream` 的 `POST /v1/chat/completions` 透传到 mock 上游（`httptest.Server`），透传前按 `ModelEntry.contextWindow/maxTokens` 做 `max_tokens` 占位钳制，响应经 `StreamTee` 模拟写入 `modernc/sqlite` 的 `requests` 表（`ts/provider/model/success/prompt/completion/cached/cost/conversation_id/latency`），`GET /api/stats` 可回查，单测覆盖 `cost` 为 `nil` 的旧行兼容。验证 `curl POST /v1/chat/completions → 200 + usage → GET /api/stats` 有行。

**Blocked by:** 09 Go 骨架与构建基座

**Status:** ready-for-agent

- [ ] `POST /v1/chat/completions`（`model=gpt-4o-mini`）透传成功：请求体原样转发 mock 上游并返回 `choices + usage`，`x-conversation-id`/`x-opencode-session` 无则记 `unlabeled`
- [ ] `max_tokens` 钳制占位：`est=ceil(len/4)`、`available=contextWindow-est-4096`、`clamped=max(16,available)` 与 `maxTokens` 取最小的逻辑可单测（覆盖 `requested` 超限重写与未超限保留）
- [ ] `requests` 表写入：每条代理请求追加一行，`cost` 来自存根计算（`(prompt-cached)*input + cached*cacheRead + completion*output`，缺单价则 `NULL`）
- [ ] `GET /api/stats` 返回最近 20 行含 `cost`（`NULL` 序列化为 `null`，前端显示 `-`）；旧行无 `cost` 列可查询不报错
