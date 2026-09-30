# 客户端取消后上游继续执行
Status: resolved

## Requirements
- 首次非流式、stream 和 retry 共用的 outbound builder 继承客户端 context。
- 客户端取消或下游写失败必须停止读取/写入并关闭 upstream body；取消记失败，不能发成功 DONE/completed。
- 非流式 body读取失败不作为成功响应，retry取消不回原400冒充结果。
- 保留现有 URL/header/timeout/重试政策；nil context 的既有 builder 调用仍可用。

## Validation
服务端全量与9条取消/写失败回归通过。Standards 的取消终止帧同批返回发现已先红后绿修复并复核；Spec 无发现。
