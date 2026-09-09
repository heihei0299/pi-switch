Status: ready-for-agent
Slug: remove-rust-core
ADR: 0007

# 移除 Rust 核心，转为 Go 单一实现

## 1. Problem Statement

项目长期双核并存（Go `internal/*` 与 Rust `src-rust/*` + napi `.node`），构建链路（`cargo` + `napi` + `go`）、发布产物（Go 二进制 + `.node` + `webui/dist`）、文档与 CI 均需双轨维护，且 `internal/` 已于 0.4 阶段对 Rust 20 个模块完成对等实现。因双轨易漂移、增加心智与维护成本，需收敛为 Go 单一实现，并以最小风险完成彻底清除。

## 2. Solution

彻底移除 Rust 工具链及关联产物，保留 Go 为唯一核心与 `src/*.js` 轻量扩展层。功能上不改变既有面向用户的行为：供应商（`config.json: profiles`）仍为唯一事实来源，网关（`models.json: providers[prefix]`）仍仅经显式发布落盘，代理仍按模型名路由并支持同模型 failover 与 Responses 透传/转换。发布以 minor 升级并附迁移说明，旧 `.node` 不再提供。

### 功能需求

- F1：仓库中不再存在 Rust 源码与构建配置：`src-rust/`、`Cargo.toml`/`Cargo.lock`、`build.rs` 必须不存在
- F2：不再发布或引用 napi 产物：预编译 `.node`、`pi-switch-native.cjs`、`index.js`/`index.d.ts` 的 napi 导出不存在，`package.json` 的 `files` 与 `scripts` 不再包含 Rust/napi 相关条目
- F3：构建链路仅保留 Go：`npm run build` 等价于 `build:webui + build:go`，`npm test` 仅跑 `go test` 与 `webui vitest`，不依赖 `cargo`/`napi`
- F4：CI 与文档同步收敛：CI 工作流、README/BUILD_MUSL/WEBUI_GUIDE 等文档中的 Rust 构建说明被移除或改为 Go 说明
- F5：保留 `src/*.js`（core/commands 等）供 pi 扩展轻量调用，本次不删；扩展仍可不依赖 daemon 直接读写配置
- F6：Go 按功能清单完成对等性门禁：Rust 20 模块（proxy failover/limit/clamp、translator、stats/usage tee、gateway publish、scan sessionScan、daemon、web API、TUI、package/ccswitch/credits/catalog/presets 等）逐项在 Go 侧可映射且现有测试全绿，方可合入
- F7：版本与迁移：以 minor 升级发布，Release Notes 明确说明破坏性——旧 `.node` 路径失效，`pnpm global` 用户需重装，全局 `pi-switch` 命令由 Go 二进制接管

## 3. User Stories

1. As a 维护者, I want 仓库仅保留 Go 单一核心, so that 构建、发布与代码审查无需双轨对比
2. As a 贡献者, I want `npm run build` 与 CI 不再依赖 Rust 工具链, so that 本地与 CI 环境一致且无需安装 `cargo`
3. As a 用户, I want 通过 `npm install -g` 仅获得 Go 二进制与 WebUI, so that 安装体积与平台矩阵简化且无 `.node` 兼容问题
4. As a pnpm global 用户, I want 升级说明明确告知 Rust 产物移除与重装步骤, so that 旧 store 快照中的 `.node` 失效时可自助恢复
5. As a pi 扩展用户, I want 扩展在移除后仍可通过 `src/*.js` 轻量管理供应商与网关发布, so that 无 daemon 时基础能力可用

## 4. Implementation Decisions

- D1：遵循 ADR 0007 的 Go-only 决策，视为已定约束，不在实现中重新权衡双轨方案
- D2：删除判定以“不存在性”为验收口径，而非“注释掉”或“空壳保留”，避免残留引用与混淆
- D3：范围严格限定为 Rust 全链路（源码、构建配置、napi 产物、CI/文档引用），`src/*.js` 与 `extensions/` 保持现状，避免本次引入扩展重构风险
- D4：构建与发布收口至 pure Go（`modernc.org/sqlite` 无 CGO），跨编译矩阵保持 `GOOS×GOARCH` 纯静态二进制，简化 `bin/pi-switch.js` 分发逻辑
- D5：以既有测试套件为最高 seam，不为删除行为新增业务接口测试；仅新增“产物不存在性”断言作为防回退门禁
- D6：版本策略采用 minor 升级承载破坏性变更（0.x 阶段 minor 即 breaking），配合迁移说明而非静默 patch，降低用户升级惊讶度

## 5. Testing Decisions

- Seam：单一最高 seam —— 既有 `go test ./...` 与 `NODE_ENV=test webui vitest` 全绿 + 仓库不存在性断言（`src-rust/`、`Cargo.*`、`build.rs`、`.node`、`pi-switch-native.cjs`、`index.js` napi 导出均不存在）
- 不新增面向用户的业务行为测试；删除行为本身无行为接口，回归由既有代理/网关/统计/扫描等模块测试覆盖
- 准入条件：功能清单逐项对照（Rust 20 模块在 Go 侧可映射）且全量测试绿，方可合入；任一失败则阻断发布
- CI 侧验证：工作流在无 `cargo` 环境下仍可完成 `build` 与 `test`，不存在 Rust 步骤残留

## 6. Out of Scope

- `src/*.js` 的进一步精简或迁移至 Go HTTP API
- `extensions/index.ts` 的重构（仍通过 `src/commands` 轻量调用，不改为纯 Go API 调用）
- 将网关与供应商真正拆为独立进程（ADR 0006 已预留接口，本次不拆）
- Rust 产物的兼容 shim 或空 re-export 保留
- 对 `requests.log`/`requests.db` 历史数据的迁移或格式变更

## 7. Further Notes

- 术语遵循 `CONTEXT.md`：供应商、上游、网关、网关发布、代理、请求日志、对话、模型目录 等保持既有定义；本文不新增 glossary 术语
- 尊重 ADR 0001/0003/0004/0006 等既有决策，移除过程不改变 tee 解析、推理 token 子集、Responses 透传、供应商-网关隔离等语义
- 发布后需同步更新 `README` 与 `WEBUI_GUIDE` 的安装/构建章节，并验证 `npm pack --dry-run` 产物不含 Rust 相关文件
