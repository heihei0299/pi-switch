# Anthropic 缓存输入与费用低估
Status: resolved

## Requirements
- Anthropic 总输入为 input_tokens + cache_creation_input_tokens + cache_read_input_tokens；普通 Chat / Responses 的 input 总量不重复累加 cached 子集。
- 非流式、SSE message_start / message_delta 和跨协议响应使用同一归一化规则；缺失与明确零值继续区分。
- 缓存创建按已有 ModelCost.CacheWrite、缓存读取按 CacheRead、普通输入按 Input 定价。
- 公共 HTTP 请求事实、Stats 总输入及缓存率与 upstream 原始 usage 一致。

## Validation
- 非流式与 SSE public HTTP 原路径红测复现 input 100 / 缓存率 700%，修后 input 1000 / 缓存率 70% / cost 0.00201。
- usage/proxy/translator/server 包通过，覆盖 Responses 子集、分帧与 delta、跨协议 round-trip、零价、缺分项和溢出。
- Standards / Spec 原发现的缺缓存创建误作零已修复；两者复核通过。
