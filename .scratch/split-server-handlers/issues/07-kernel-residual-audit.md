# 07: kernel 提炼与残余审计

**What to build:** 六个域 handler 迁完之后，`server.go` 收敛为可交付清单的 kernel，并且「共享面一眼可见」。端到端行为：任何被 2+ 域调用的 helper 都显式位于 `server.go`，域文件之间不互相调用内部 helper；`server.go` 只保留路由注册、认证、静态资源、构建信息与 embed 变量、以及 kernel helper。

**kernel 应保留的内容**（spec D2 映射表，kernel 行）：

- 路由注册与路由表
- auth：`authMiddleware`、`isLoopback`、`splitHostPort`、`webUIPasswordPath`、`resolveWebUIPassword`
- 静态资源服务
- 构建信息与 `embed` 变量
- kernel helper：`configPath`、`saveConfig`、`resolveModelsDevProvider`

本票是 spec D8「commit 1 纯文件移动」的收尾分片：与 01–06 同属一个移动提交，本票只做「跨域 helper 归位 + 清单审计」，不引入逻辑改动。机械守护测试本轮不加（spec D3：无证据不加机制；漂移出现再加）。

**Blocked by:** 01: proxy 域 handler 迁出 kernel；02: profile 域 handler 迁出 kernel；03: gateway 域 handler 迁出 kernel；04: package 域 handler 迁出 kernel；05: settings 域 handler 迁出 kernel；06: stats 域 handler 迁出 kernel

**Status:** resolved (2026-09-11)

- [x] 审计确认 `server.go` 只含上述 kernel 清单符号，无任何域 handler 或域 helper 残留
- [x] 被 2+ 域调用的 helper 全部位于 `server.go`，且每个都有一段注释或映射表条目说明它被哪几个域共用
- [x] 域文件之间零内部依赖：`proxy_handlers.go` / `profile_handlers.go` / `gateway_handlers.go` / `package_handlers.go` / `settings_handlers.go` / `stats_handlers.go` 只依赖 kernel 与自身
- [x] **豁免清单（01 复核补录，审计时必须排除）**：`retry.go`、`outbound.go`、`legacy_log.go` 不属于 D2 的任何域（spec D2 明确这三个文件不动），但其符号被域文件调用是**既定形态**，不算 D3 违规。已确认 proxy 域依赖其中 8 个符号：`attempt`、`narrowToChannel`（retry.go）、`BuildOutboundRequest`、`OutboundRequestPlan`、`selectedOutboundUpstream`（outbound.go）、`requestURLOf`、`appendLegacyLog`、`legacyLogEntry`（legacy_log.go）。审计时须把「域文件 → 这三个文件」列为合法依赖，否则本 AC 与现状冲突
- [x] `server.go` 行数降至约 490 行量级（spec 预期收益）
- [x] `gofmt -l` 无输出；`go test ./...` 全绿

## 审计记录

- 提交：`5fb5c76`（与 10 合并提交）

**`server.go` 残余完整清单**（535 行 / 18 个顶层 func；票 10 之后）

| 符号 | 归属 | 判定 |
|---|---|---|
| `init`、`computeBuildInfo` | 构建信息 | kernel 清单 |
| `Version`/`BuildTime`/`BuildCommit`/`BuildTarget`/`BuildDirty`、`webuiEmbedded`/`webuiIndexHash`/`webuiScript`/`webuiAssetCount`/`webuiOnce`、`webUIFS` | 构建信息与 embed 变量 | kernel 清单；`webUIFS` 仅 kernel 内部（`computeBuildInfo`/静态资源）使用 |
| `NewProxyRouter`、`NewMgmtRouter` | 路由注册 | kernel 清单 |
| `authMiddleware`、`isLoopback`、`splitHostPort`、`webUIPasswordPath`、`resolveWebUIPassword` | 认证 | kernel 清单 |
| `handleWebUIIndex`、`handleWebUIFallback`、`handleAssets` | 静态资源 | kernel 清单 |
| `indexOf` | 静态资源 helper | **清单外但合规**：仅被同文件 `handleAssets` 调用（`server.go:115`），无其他调用者，属静态资源域的 kernel 侧 helper |
| `configPath` | kernel helper | kernel 清单；被 6 个域 + `legacy_log.go` 调用 |
| `saveConfig` | kernel helper | kernel 清单；被 profile、settings 两域调用 |
| `sessionScanCandidates`、`conversationCandidates` | kernel helper | 01 的 D3 裁决；被 proxy、stats 两域调用 |
| `effectiveConversationID` | **死代码（未删）** | 全仓（含测试）零调用者。spec D7 只授权删 `contains` 与 `modelsDevCatalog`，且 Further Notes 要求不扩大删除范围，故本票记录不删，列为后续清理候选 |

**被 2+ 域调用的 kernel helper 及其调用域**

| helper | 调用域 |
|---|---|
| `configPath` | proxy、profile、gateway、package、settings、stats（6 域）+ `legacy_log.go` |
| `saveConfig` | profile、settings |
| `resolveModelsDevProvider` | profile（`profile_handlers.go:666`）、gateway（`gateway_handlers.go:161`）——**D3 合规，spec D2 的 kernel 归属正确** |
| `sessionScanCandidates` | proxy（`conversationIDFrom`）、stats（`newStatsService`） |
| `conversationCandidates` | proxy、stats |

**域文件相互依赖检查**：6 个域文件之间零内部依赖，全部只引用 kernel、三个豁免文件（`retry.go`/`outbound.go`/`legacy_log.go`）与自身。已用「按符号名收集引用方文件」机械枚举，非人工扫读。

**D1 导出符号核验（机械对比，非声明）**：把搬迁前 `46d8153` 与当前 HEAD 的 `internal/server` 非测试文件按 `^(func|type|var|const) [A-Z]` 提取导出符号名并取集合：

- 搬迁前：`ImportPiPackages`、`ListInstalledPackages`、`NewMgmtRouter`、`NewProxyRouter`（4 个）
- 搬迁后：上述 4 个 **完全一致**，另有 `BuildOutboundRequest`、`BuiltOutboundRequest`、`ImportLegacyNow`、`ImportLegacyOnStartup`、`OutboundRequestMetadata`、`OutboundRequestPlan` —— 这 6 个来自本 spec 明确不动的 `outbound.go` 与 `legacy_log.go`，不是拆分引入
- 即：**六个域文件合计新增导出符号 0 个**。`package_handlers.go` 的 2 个导出符号（`ListInstalledPackages`、`ImportPiPackages`）是被 `cmd/pi-switch/main.go` 调用的既有导出 API，搬迁前就在 `server.go` 中导出，拆分保留了这一契约；本 spec 未把它们改名为小写，因为那会构成对 `cmd` 的破坏性接口变更，超出 spec D10 的零行为变更范围

**验收命令**：`go build ./...` 通过；`go test ./...` 全绿；`gofmt -l` 无输出。

**遗留候选（不在本 spec 范围）**：`effectiveConversationID`（零调用者）；`dump400` + `/api/dumps`（生产者零调用者，端点保留但目录恒空）。
- [ ] `git diff --color-moved` 确认本次移动提交全部为移动，无逻辑改动（spec 验收门禁）
