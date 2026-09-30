# Anthropic SSE 跨协议错误透传
Status: resolved

## Requirements
- Anthropic↔Chat 未实现 SSE 转换时，在访问 upstream 前返回明确 502 not_supported。
- 不把无转换器等价为同协议 passthrough；同协议及 Chat↔Responses SSE 保持。
- Anthropic↔Chat 非流式转换继续正常。

## Validation
server/translator 全部通过；两轴审查无发现。
