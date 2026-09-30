# 非流式取消与正常 EOF 同时发生时误记成功
Status: resolved

## Requirements
- 沿用 HTTP 代理入口与 Stats API 回归边界；覆盖首次请求、max-token retry、同协议与跨协议响应。
- upstream body 最后一次 Read 返回完整响应与 io.EOF，同时客户端 context 被取消时，停止后续转换和写入并关闭 body。
- Stats 只记录一次失败，status 为 499，error 保留 context cancellation；不得记录成功或回退为首次 400。
- 正常非流式响应、真实读取失败和既有重试策略保持；遵循 system-contract §2.4.10。

## Validation
- `GOFLAGS='-run=^TestNonStreamingCancellationAtEOFIsRecordedAsFailure$' scripts/test-limited.sh go`：四种 HTTP/Stats 场景实际 Red（取消后正文、成功 200），修复后 Green。
- 同脚本选跑既有取消、SSE 写失败、终止帧、非流式协议/转换错误及请求状态事实回归，均通过。
- Standards 与 Spec 提交前双轴审查均无代码发现；已同步 issue 状态和验证记录。
