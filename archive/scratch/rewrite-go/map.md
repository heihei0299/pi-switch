# Wayfinder Map: pi-switch Go 重写

## Destination

将 pi-switch 从 Rust + TypeScript 技术栈迁移到 Go，参照 CLIProxyAPI 的模块化架构重新实现。保留 pi-switch 面向 pi agent 的核心功能（provider profile 管理、gateway 路由、failover、统计/WebUI、TUI、package 管理），同时借鉴 CLIProxyAPI 的设计思想（清晰的 internal/ 模块划分、协议转换器、管理 API、可嵌入 SDK）。最终产出是一个纯 Go 编写的、功能等价或增强的 pi-switch v2。

## Notes

- **参考技能**: wayfinder, grilling, domain-modeling, code-review
- **参考仓库**: CLIProxyAPI (github.com/router-for-me/CLIProxyAPI) — Go, gin, 模块化设计
- **现有仓库**: pi-switch (本地) — Rust (axum, napi-rs) + TypeScript/React (WebUI)
- **约束**: 保留包名 @heihei0299/pi-switch 和发布线；不破坏现有用户的 config.json 结构（至少提供迁移路径）
- **范围纪律**: 一次只处理一个 ticket；不跳跃执行

## Decisions so far

- [Go 框架与项目结构](issues/01-go-framework.md): 采用 gin + CLIProxyAPI 式标准布局 (`cmd/`, `internal/`, `pkg/`, `webui/`, `sdk/`)，独立守护进程 + HTTP API 与 pi 集成，不引入插件生态。
- [配置系统迁移](issues/02-config-system.md): 保留 JSON 为主，per-request 读取，Go 中保持现有 schema 兼容，旧字段自动迁移，struct tag + 自定义校验。
- [认证架构](issues/03-auth-architecture.md): 仅 api-key + 预留 AuthProvider 扩展点，首版不实现 OAuth，不引入 auth-dir / 客户端 api-keys，管理 API 沿用 basic auth。
- [WebUI 与管理 API](issues/04-webui-management-api.md): 保留 React WebUI，Go 用 embed.FS 嵌入 webui/dist，管理 API 保留现有 REST 规范化为 /api/*，basic auth。
- [代理核心与协议转换](issues/05-proxy-translator.md): 直接复用 CLIProxyAPI translator 包，internal/proxy 拆为 router/failover/convert/forward/limit/stream，流式与限流沿用现有语义，failover 保留逐候选尝试。
- [统计与存储](issues/06-stats-store.md): 保留 SQLite + requests.log，Go 用 modernc.org/sqlite（纯 Go），用量解析与查询导出 1:1 移植，保留 sessionScan。
- [构建发布与迁移](issues/07-build-release-migration.md): 两阶段构建（webui + go build）多平台交叉编译，npm 包装 Go 二进制发布，~/.pi-switch 零改动兼容，复用 daemon pid 逻辑。
- [TUI](issues/08-tui.md): 用 charmbracelet/bubbletea 重写 ratatui，功能 1:1，作为 `pi-switch tui` 子命令。
## Not yet specified

<!-- 已清空 — 全部决策已分票并解决 -->
## Out of scope

- 不替换 pi agent 本身（只替换 pi-switch 代理层）
- 不重新设计 opencode-go 上游协议（保持兼容）
- 不引入 CLIProxyAPI 的 Home 模式/集群模式（超出 pi-switch 单用户定位）
- 不引入 WebRTC、Redis Queue、Pion 等 CLIProxyAPI 的通信基础设施（除非明确需要）
- 不引入 CLIProxyAPI 插件生态（用户已确认仅需核心功能）
- 插件/扩展系统：pi 现有 TypeScript 扩展（conversation-id-inject 等）待评估与 Go 后端适配（已移至单独评估）
- SDK 嵌入：CLIProxyAPI 的可嵌入 Go SDK — 首版不需要，仅独立守护进程 + HTTP API（决策 2026-09）。
