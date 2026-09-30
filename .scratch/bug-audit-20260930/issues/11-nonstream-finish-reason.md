# 非流式 Chat 响应转 Responses 丢失结束原因
Status: resolved

## Requirements
- 沿用 HTTP 代理回归边界；Responses 客户端访问 Chat upstream 时，首次及 max-token retry 都保留结束原因。
- finish_reason=length 映射为 status=incomplete、incomplete_details.reason=max_output_tokens；content_filter 映射为 incomplete/content_filter。
- 截断的 message/function_call output item 标记 incomplete，保留文本、调用 ID 和已有参数；正常 stop/tool_calls 仍 completed。
- 非流式与现有 SSE 使用同一结束原因映射；遵循 system-contract §2.4.7/8，保留既有 SSE 事件语义。

## Validation
- `GOFLAGS='-run=^TestChatFinishReasonReachesResponsesClient$' scripts/test-limited.sh go`：首次/retry 的 length 文本、content_filter 文本、length 工具共六场景实际 Red；正常 stop/tool_calls 对照保持通过，修复后十场景 Green。
- 同脚本选跑现有 Responses 请求/响应、SSE、非流式协议及转换失败回归，均通过。
- Standards 与 Spec 提交前双轴审查均无发现。
