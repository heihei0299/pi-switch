# 0007 移除 Rust 核心，转为 Go 单一实现

项目自 0.4 起 Go 已实现与 Rust 对等的完整能力（代理转发与同模型 failover、限流 clamp、Responses 透传/转换、网关发布、统计与 tee 解析、会话归因 sessionScan、daemon/web API、TUI、供应商/模型管理、package/ccswitch/credits/catalog/presets），双核并存增加构建、发布与维护成本且易漂移。决定：彻底移除 Rust 工具链——删除 `src-rust/`、`Cargo.toml`/`Cargo.lock`、`build.rs`、预编译 `.node` 产物、`pi-switch-native.cjs` 与 `index.js`/`index.d.ts` 的 napi 导出，以及 CI/脚本/文档中的 Rust 引用；保留 `src/*.js` 供 pi 扩展轻量调用；`npm run build` 与 CI 仅保留 `build:webui + build:go` 路径；发布以 minor 升级并附迁移说明（旧 `.node` 不再提供，pnpm global 用户需重装）。

## Considered Options
- 保留 Rust 作为 napi 备选双构建：可平滑过渡但持续双轨维护与双二进制发布成本高，且 Go 已通过功能清单对照验证完备，否决
- 仅删 `src-rust/` 源码、保留 Cargo 空壳与 shim：残留工具链与空导出增加混淆，不彻底，否决
- 同步一并清除 `src/*.js`：虽可进一步统一至 Go HTTP API，但会扩大为扩展重构，本次范围仅清 Rust，留待另起 feature，否决

## Consequences
- 构建与发布仅依赖 Go（`modernc.org/sqlite` pure Go，无 CGO），跨编译与安装链路简化；`npm pack` 不再包含 `.node`
- 既有通过 `index.js` 直接 `require('pi-switch-native')` 的外部调用失效，需迁移至 Go 二进制或 Web API
- 需以功能清单逐项对照 Rust 20 模块并保证 `go test ./...` 与 `webui vitest` 全绿作为移除准入，未通过不得合入
