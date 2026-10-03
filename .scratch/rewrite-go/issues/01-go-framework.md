## Question

pi-switch Go 重写应选择哪个 HTTP 框架和项目目录结构？

CLIProxyAPI 使用 gin + 清晰的 `internal/` 模块划分（api, auth, config, translator, client, store 等）。pi-switch 现有 Rust 代码使用 axum，模块划分相对扁平（proxy.rs, stats.rs, config.rs 等，proxy.rs 达 224KB）。

需要决策：
1. HTTP 框架：gin（与 CLIProxyAPI 一致）/ echo / fiber / 标准库 net/http？
2. 项目结构：是否采用 CLIProxyAPI 式的 `cmd/`, `internal/`, `pkg/`, `sdk/` 分层？
3. 模块粒度：proxy.rs 的 224KB 巨文件如何拆分到 Go 的 internal/ 子包中？
4. 与 pi 的集成方式：现有通过 napi-rs 暴露 Rust 函数给 Node.js，Go 重写后 pi 如何调用？纯 HTTP API？Go 嵌入 Node.js（如 go-napi）？还是 pi-switch 变为独立守护进程？

## Type

grilling

resolved

## Answer

1. **HTTP 框架**: gin — 与 CLIProxyAPI 一致，生态成熟，api 风格与现有 axum 最接近。
2. **项目结构**: CLIProxyAPI 式标准布局 — `cmd/`, `internal/`, `pkg/`, `webui/`, `sdk/` 分层。proxy.rs 拆分为 `internal/proxy/` 子包（router, failover, convert, forward, limit, stream）。
3. **与 pi 集成**: 独立守护进程 + HTTP API — Go 编译为独立二进制，`pi-switch proxy/webui/tui` 作为子命令运行。不再依赖 napi-rs/CGO。
4. **范围约束**: 不引入 CLIProxyAPI 的插件生态系统，只借鉴其核心代理功能架构。
