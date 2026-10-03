## Question

Go 重写的认证架构应如何设计？是否需要引入 CLIProxyAPI 式的多供应商 OAuth？

pi-switch 现有认证以 api-key 为主（provider profile 中配置 api_key），面向 pi agent 的单一用户场景。CLIProxyAPI 支持多供应商 OAuth（Codex、Claude Code、Grok Build）和 api-key 混合，面向多 CLI 账户的通用代理场景。

需要决策：
1. 认证模式：保留纯 api-key / 引入 OAuth 支持 / 两者并存？
2. OAuth 范围：如果引入，支持哪些供应商？（Codex、Claude、Grok、Gemini、Kimi？）
3. 账户管理：CLIProxyAPI 的 auth-dir（~/.cli-proxy-api）存储多账户 token，pi-switch 是否需要同等机制？
4. api-keys 列表：CLIProxyAPI 支持多个 api-keys 做客户端认证，pi-switch 现有无此机制，是否需要？
5. 与 pi agent 的集成：pi agent 通过 pi-switch 代理时，认证信息如何传递？现有是通过 profile 的 api_key 转发上游，OAuth 模式下如何转发？

## Type

grilling

## Status

resolved

## Answer

1. **认证模式**: 仅 api-key + 预留扩展点 — 首版保留现有 `profile.api_key` 转发上游的模式，Go 中设计 `AuthProvider` 接口预留 OAuth 扩展，但首版不实现具体 OAuth 流程。
2. **OAuth 范围**: 首版不实现；接口设计时考虑 Codex/Claude/Grok 的扩展可能性。
3. **账户管理**: 首版不引入 auth-dir 多账户存储，沿用 `~/.pi-switch/config.json` 单文件；OAuth 引入时再评估独立 auth 存储。
4. **api-keys 列表**: 首版不引入客户端 api-keys 列表（pi-switch 为本地单用户代理，无需客户端认证）；管理 API 的认证沿用现有 WebUI 的 basic auth。
5. **与 pi 集成**: 保持现有转发模式 — gateway 按 profile 的 api_key / headers 转发上游；AuthProvider 接口抽象转发逻辑。
