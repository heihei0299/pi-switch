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

**Status:** ready-for-agent

- [ ] 审计确认 `server.go` 只含上述 kernel 清单符号，无任何域 handler 或域 helper 残留
- [ ] 被 2+ 域调用的 helper 全部位于 `server.go`，且每个都有一段注释或映射表条目说明它被哪几个域共用
- [ ] 域文件之间零内部依赖：`proxy_handlers.go` / `profile_handlers.go` / `gateway_handlers.go` / `package_handlers.go` / `settings_handlers.go` / `stats_handlers.go` 只依赖 kernel 与自身
- [ ] **豁免清单（01 复核补录，审计时必须排除）**：`retry.go`、`outbound.go`、`legacy_log.go` 不属于 D2 的任何域（spec D2 明确这三个文件不动），但其符号被域文件调用是**既定形态**，不算 D3 违规。已确认 proxy 域依赖其中 8 个符号：`attempt`、`narrowToChannel`（retry.go）、`BuildOutboundRequest`、`OutboundRequestPlan`、`selectedOutboundUpstream`（outbound.go）、`requestURLOf`、`appendLegacyLog`、`legacyLogEntry`（legacy_log.go）。审计时须把「域文件 → 这三个文件」列为合法依赖，否则本 AC 与现状冲突
- [ ] `server.go` 行数降至约 490 行量级（spec 预期收益）
- [ ] `gofmt -l` 无输出；`go test ./...` 全绿（需用户明确授权编译/测试命令）
- [ ] `git diff --color-moved` 确认本次移动提交全部为移动，无逻辑改动（spec 验收门禁）
