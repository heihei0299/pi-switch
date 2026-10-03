## Question

Go 重写中代理核心（gateway 路由、failover、协议转换）如何实现？直接复用 CLIProxyAPI 的 translator 还是移植 pi-switch 现有逻辑？

pi-switch 现有 `proxy.rs` (224KB) 包含：gateway 按 model 名称路由、failover 链、OpenAI↔Anthropic↔Responses 转换、contextWindow 限流、SSE/WebSocket 流式、StreamTee 用量解析。CLIProxyAPI 的 `internal/translator` + `sdk/translator` 提供了更模块化的 OpenAI/Claude/Gemini 互转实现。

需要决策：
1. 协议转换：复用 CLIProxyAPI 的 translator 包 / 移植 pi-switch 现有 convert 逻辑 / 混合？
2. 模块拆分：`internal/proxy/` 下如何拆文件（router.go, failover.go, convert.go, forward.go, limit.go, stream.go）？
3. 流式与限流：SSE 流、contextWindow 估算、max_output_tokens/max_tokens 钳制逻辑如何迁移？
4. Failover 语义：保持现有“按候选 profile 逐个尝试”的语义，还是借鉴 CLIProxyAPI 的重试/熔断策略？

## Type

grilling

## Status

resolved

## Answer

1. **协议转换**: 直接复用 CLIProxyAPI 的 `internal/translator` / `sdk/translator` 包，适配 pi-switch 的 gateway 按 model 名称路由语义；按需裁剪 Gemini 相关依赖，保留 OpenAI↔Anthropic↔Responses 核心路径。
2. **模块拆分**: `internal/proxy/` 下拆为 router.go（路由分发）、failover.go、convert.go（封装 translator 调用）、forward.go、limit.go（contextWindow 钳制）、stream.go（SSE/WebSocket）。
3. **流式与限流**: SSE/WS 流式与 max_output_tokens/max_tokens 钳制逻辑沿用现有 pi-switch 语义，封装在 limit.go/stream.go 中，复用 translator 的流式转换能力。
4. **Failover**: 保留现有“按 failover 链逐候选尝试”语义，熔断作为后续可选增强，不在首版引入。
