# 跨协议工具调用丢失
Status: resolved

## Requirements
- Chat↔Responses 与 Chat↔Anthropic 支持函数工具定义、选择策略和完整历史（调用 ID、参数、结果），响应工具调用也要保留。
- 不生成空 content 的 Responses assistant 消息占位；纯工具调用由 function_call 项承载。
- 对缺失 ID/name、非法参数或不支持的工具类型明确报错；运行时使用返回 error 的转换入口。
- 保持既有文本请求与限制/模型/header策略；不引入 Anthropic SSE 转换功能。

## Validation
转换器及 HTTP 回归通过；Standards 的空占位与 Spec 的反向无效历史发现均经回归修复，复核无遗留发现。
