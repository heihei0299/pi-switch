# 06: 修正 `docs/architecture-review.md` A7

Status: resolved (2026-09-11)

**两处不准确**：
1. 改写版把「本项判据在全仓范围仍不成立」的结论删掉了，而 `POST /api/gateway/start` 依然返回
   `running:true` 且无副作用（有意设计）——判据仍不成立，必须写明，否则读者会以为已满足。
2. 「不再按错误文本分类」只对 handler 成立：`daemon.Start` 仍按日志文本判定端口占用。

**验收**
- [ ] A7 明确写出「全仓判据仍不成立」及其原因，不再读作已满足
- [ ] 「不再按文本分类」限定到 handler，并说明 daemon 侧为何只能读日志
- [ ] 三个遗留端点的处置与新文档一致（501 / 早退 / 真实文件检查 / gateway start 有意不改）
