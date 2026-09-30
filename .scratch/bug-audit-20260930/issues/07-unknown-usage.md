# 未知 usage 被伪造为零
Status: resolved

## Requirements
- raw upstream usage 的已知/缺失状态保留到 SQLite、legacy log、Stats 请求及对话明细、导出；明确零值仍是零。
- 首次、retry 和两种stream relay使用同一 UsageSummary；统计读raw usage，不能被协议转换丢掉细节。
- 无usage/部分usage不伪造输入输出或cost；缺少缓存分布且价格依赖缓存时cost为unknown。
- 总体、provider、model、conversation缓存率只用cached已知的成功完整usage样本；未知不稀释分母，无样本显示unknown。
- 保留既有聚合字段/分页/归属规则；总token聚合仍只累计成功完整usage中的已知事实。

## Validation
- 原 public HTTP/SQLite/Stats/JSON/CSV 回归先失败，修复后通过；usage/proxy/stats/server/translator 包测试通过。
- Standards / Spec 审查发现单请求缓存率对失败/部分 usage 门控缺失；红测复现后改为成功完整 usage，复核均通过。
