# 03: `provider test` 接线

**What to build:** `pi-switch provider test <name>` 真实探测上游（只读、5s 超时），输出 success/message/responseTimeMs。

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] 抽出上游探测核心为可共用函数，handler 改为委托
- [ ] CLI 接线；探测失败（如错误 Key）时非零退出且不打印成功文案
- [ ] 测试：对 mock 上游断言成功与失败两条路径（**不打真实上游**）
- [ ] 不写盘、不污染统计（沿用 handler 现有语义）
