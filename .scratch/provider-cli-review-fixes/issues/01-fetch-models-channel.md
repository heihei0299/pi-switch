# 01: `provider fetch-models --channel` 渠道定向拉取

Status: resolved (2026-09-11)

**What to build:** `pi-switch provider fetch-models <name> [--channel <name>]`。带 `--channel` 时按该渠道的 baseUrl/apiKey/headers 拉取，enrich 后把新 id 合并入该渠道并落盘（与 `POST /api/profiles/:name/fetch-models?channel=` 同一实现）；不带时保持只读列出。

**为什么**：上一批次 `provider-cli-wiring/issues/04` 的原文要求 `[--channel <name>]` 且「带 --channel 时 enrich 后合并落盘」，实现只做了只读列出，checkbox 却勾上了。评审实测 `provider fetch-models p --channel nope` → rc=0 并照常打印，flag 被静默忽略。

**Seam**：CLI `handleProvider` + `provider show`（断言落盘）；服务端 `POST /api/profiles/:name/fetch-models`。

**验收**
- [ ] 带 `--channel` 拉到的模型出现在该渠道的 `models[]` 中，且只影响该渠道
- [ ] 未知渠道非零退出并说明原因（与 handler 的 400 语义一致）
- [ ] 上游失败非零退出、原因可见、配置不被改动
- [ ] 不带 `--channel` 时行为不变（只读、不写盘）
- [ ] handler 委托同一实现（`handleFetchModels` 不再有第二份 `/models` 解析）

**测试**：`cmd/pi-switch/provider_fetch_models_test.go`（mock 上游 + `provider show`）、沿用 `internal/server/channel_fetch_test.go` 作为 handler 侧回归网。
