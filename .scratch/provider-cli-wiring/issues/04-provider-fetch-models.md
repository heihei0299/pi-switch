# 04: `provider fetch-models` 接线

**What to build:** `pi-switch provider fetch-models <name> [--channel <name>]` 拉取模型列表；带 `--channel` 时 enrich 后合并落盘（与 handler 的渠道定向语义一致）。

**Blocked by:** 01

**Status:** **复审推翻**（原记 resolved 2026-09-11，复审 2026-09-11 判定勾选不实）

- [x] CLI 接线；上游失败/未知渠道时 stderr + 非零 —— 仅对**不带** `--channel` 的只读列表成立
- [x] 该命令会消耗上游额度，测试全程只用 mock
- [ ] 抽出拉取/合并核心为可共用函数，handler 改为委托（含未知渠道 400 语义）
- [ ] 带 `--channel` 时 enrich 后合并落盘
- [ ] 测试：mock 上游断言拉取结果与落盘效果（渠道定向）

**复审结论**（code-review 两轴独立得出，已实测）：本条票的原始要求是
`fetch-models <name> [--channel <name>]`，实现只做了只读列出，`--channel` 从未被解析。
实测 `provider fetch-models p --channel nope` → rc=0 并照常打印模型，flag 被静默忽略——
正是本 spec 存在的理由所要消灭的「假成功」。此外 `FetchUpstreamModelIDs` 当时只被 CLI 调用，
handler 仍保留第二份 `/models` 解析，与「不为 CLI 另写一套逻辑」相悖。

**已于批次 `provider-cli-review-fixes/issues/01` 修复**：抽出 `server.FetchChannelModels`
供 handler 与 CLI 共用，CLI 支持 `--channel`（enrich + 合并落盘），未知 flag/未知渠道一律拒绝。
