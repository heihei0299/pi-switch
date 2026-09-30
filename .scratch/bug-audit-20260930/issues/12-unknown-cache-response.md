# Chat 转 Messages 响应猜测未知的缓存输入组成
Status: resolved

## Requirements
- 沿用 HTTP 代理与 Stats API 回归边界，覆盖首次及 max-token retry。
- Chat prompt_tokens 是总输入；cached 未知时不能将其冒充 Anthropic 普通 input_tokens。沿用转换器已有的 nullable unknown 语义，将无法计算的普通输入保留为 null，不伪造缓存读取/创建为零。
- 保留已知 output_tokens；cached 明确为零或非零时，继续输出正确的普通输入与 cache read/write 组成。
- Stats 仍使用 raw upstream usage：总输入和输出已知值保留，cached 与依赖缓存分布的消费保持 unknown。
- 遵循 system-contract §2.4.7、§2.5.8/9；不引入新的失败策略或修改上游响应事实。

## Validation
- `GOFLAGS='-run=^TestMessagesUsageDoesNotGuessUnknownCache$' scripts/test-limited.sh go`：未知缓存首次/retry 实际 Red（普通 input 误写 100），修复后未知/明确零/25 缓存共六个 HTTP/Stats 场景 Green。
- 同脚本选跑现有 Usage、Cache、Anthropic、Cost 与 Stats/导出回归，均通过。
- Standards 与 Spec 提交前双轴审查均无发现。
