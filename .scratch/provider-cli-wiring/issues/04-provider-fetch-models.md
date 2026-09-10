# 04: `provider fetch-models` 接线

**What to build:** `pi-switch provider fetch-models <name> [--channel <name>]` 拉取模型列表；带 `--channel` 时 enrich 后合并落盘（与 handler 的渠道定向语义一致）。

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] 抽出拉取/合并核心为可共用函数，handler 改为委托（含未知渠道 400 语义）
- [ ] CLI 接线；上游失败/未知渠道时 stderr + 非零
- [ ] 测试：mock 上游断言拉取结果与落盘效果（渠道定向）；失败路径断言
- [ ] 该命令会消耗上游额度，测试全程只用 mock
