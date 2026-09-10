# 01: proxy 域 handler 迁出 kernel

**What to build:** 把代理核心的 handler 与 helper 从 `internal/server/server.go` 机械搬进 `internal/server/proxy_handlers.go`，代理行为零变化。端到端行为：模型列表端点、路由解析与渠道固定、流式/非流式 chat completions 代理、协议入站判定、请求日志写入、400 dump 链路全部照旧工作；改动代理逻辑时 diff 只落在一个文件里。

**归入本票的符号**（spec D2 映射表，proxy 行）：

- 入口与路由解析：`handleModels`、`resolveRoute`、`pinChannelAttempts`、`conversationIDFrom`、`findModelEntry`、`incomingProtocol`
- 请求体处理：`estimateEffectiveLen`、`clampBody`、`findFrameEnd`、`extractData`
- 代理执行：`handleChatCompletions`、`handleStream`、`streamPassthrough`、`streamConvert`、`cloneMap`
- 观测链路：`extractUsage`、`computeCost`、`logRequest`、`dump400`、`handleDumps`

（`computeCost` 本票仅随搬运易位，删除在 09。加上它共 20 个符号。）

本票是 spec D8「commit 1 纯文件移动」的第一个分片，与其他域分片共用同一个移动提交（见 07）。

**Blocked by:** None — can start immediately

**Status:** resolved (commit b3c93e8, 2026-09-11)

- [x] 上述符号全部位于 `proxy_handlers.go`，保留原 `internal/server` package，不新增子包、不导出符号（spec D1）
- [x] `server.go` 不再声明上述符号；被 2+ 域调用的 helper 不出现在 `proxy_handlers.go`，而是留在 kernel（spec D3）
- [x] 零逻辑改动：函数体、签名、注释逐字不变
- [x] 测试文件零改动：45 个 `internal/server/*_test.go` 原地不动；`retry.go`、`outbound.go`、`legacy_log.go` 不动（spec D2/D4）
- [x] 路由注册、认证、JSON 契约、WebUI 不变（spec D10）
- [x] `gofmt -l` 无输出
- [x] `go test ./...` 全绿（16 个包；`internal/server` 9.26s）
- [x] 不触碰 `computeCost`、`configPath`、`saveConfig` 的收敛，那是 08/09 的范围

## 实施记录

- 提交：`b3c93e8` — refactor(server): extract proxy handlers into proxy_handlers.go（曾 amend，旧哈希 `5da6a14` 已不可达）
- 交付：`internal/server/proxy_handlers.go`（新增 889 行 / 20 个符号）、`internal/server/server.go`（3645 → 2774 行 / 118 → 98 个符号）
- 搬迁块实测 **867 行**（2718-2804 共 87 行 + 2814-3593 共 780 行），含 19 个空行；889 = 867 + 头 22 行（package/import 20 行 + 空行 2）
- 纯移动证据（按强度排序）：
  1. **函数级**：改前 `server.go` 的 115 个可解析函数逐个提取函数体，与改后两文件比对 → IDENT 115 / CHANGED 0 / LOST 0 / DUP 0；每个符号恰好声明一次
  2. `server.go` 侧 diff 为 0 插入 / 852 行删除（871 行含空行，差额 19 为搬迁空行）；两侧严格互补
  3. 行级：删除内容与 `proxy_handlers.go` 正文（忽略空行）逐行一致，唯一差异是随搬迁移出的 4 个 import（`math`、`limit`、`translator`、`usage`）
- 验收命令：`gofmt -l internal/server/` 无输出；`go test ./...` 全绿
- **D3 裁决（偏离 spec D2 表格）**：`sessionScanCandidates` 与 `conversationCandidates` 留在 kernel，未随 06 进 `stats_handlers.go`。理由：被两个域调用（proxy 的 `conversationIDFrom`、stats 的 `newStatsService`），按 spec D3「被 2+ 域调用的 helper 必须位于 kernel」应留驻。06 需同步这条裁决
- **D3 依赖面（复核补录，07 必读）**：`proxy_handlers.go` 除 kernel 的 `configPath` 与上述两个候选 helper 外，还依赖 **`retry.go`**（`attempt`、`narrowToChannel`）、**`outbound.go`**（`BuildOutboundRequest`、`OutboundRequestPlan`、`selectedOutboundUpstream`）、**`legacy_log.go`**（`requestURLOf`、`appendLegacyLog`、`legacyLogEntry`）共 8 个符号。这三个文件不属于 D2 的任何域（spec D2 明确「`retry.go`/`outbound.go`/`legacy_log.go` 不动」），但它们是域文件的既有依赖。**07 的「域文件只依赖 kernel + 自身」这条 AC 必须把它们列为明确豁免**，否则审计会与现状冲突并误导后续实现者
- 事实修正：`computeCost` 在 `proxy_handlers.go` 中是 **6 个调用点**（spec D6 的计数正确）；本票早期记录的「实际 5 个」源于一处用 `\b` 词边界匹配的 grep 漏掉了嵌套分支内的一处
