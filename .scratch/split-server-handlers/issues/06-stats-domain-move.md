# 06: stats 域 handler 迁出 kernel

**What to build:** 把统计域的 handler 与查询 helper 从 `internal/server/server.go` 机械搬进 `internal/server/stats_handlers.go`，统计行为零变化。端到端行为：统计概览、会话列表、单会话请求分页、日志导出、时间窗口解析与范围归一化全部照旧工作；回归定位时统计改动不再与代理/包管理混在同一 diff。

**归入本票的符号**（spec D2 映射表，stats 行）：

- 窗口与分页：`normalizeRange`、`window`（类型）、`parseWindowQuery`、`statsWindowFor`、`statsPageLimit`
- 服务与入口：`newStatsService`、`handleStats`、`handleStatsConversations`、`handleConversationRequests`、`handleLogsExport`
- 候选扫描：`effectiveConversationID`（`sessionScanCandidates`、`conversationCandidates` 见下）

**前置裁决（01 已定，本票受约束）**：`sessionScanCandidates` 与 `conversationCandidates` **不进 `stats_handlers.go`，留在 kernel**。理由：它们被两个域调用——本票的 `newStatsService` 与 proxy 的 `conversationIDFrom`——按 spec D3「被 2+ 域调用的 helper 必须位于 kernel」应留驻。spec D2 表格把它们列在 stats 行，但 D3 规则优先（spec Further Notes：清单漂移以「域归属 + kernel 规则」为准，不扩大范围）。

本票是 spec D8「commit 1 纯文件移动」的分片之一，与其他域分片共用同一个移动提交（见 07）。

**Blocked by:** 01: proxy 域 handler 迁出 kernel

**Status:** ready-for-agent

- [ ] 上述符号与 `window` 类型全部位于 `stats_handlers.go`，保留原 `internal/server` package，不新增子包、不导出符号（spec D1）；`sessionScanCandidates`/`conversationCandidates` 按上面前置裁决留在 kernel
- [ ] `server.go` 不再声明上述符号；日志导出若与 proxy 域日志 helper 有交集，以 kernel 规则裁决（spec D3）
- [ ] 零逻辑改动：函数体、签名、注释逐字不变
- [ ] 测试文件零改动；`retry.go`、`outbound.go`、`legacy_log.go` 不动（spec D2/D4）
- [ ] 路由注册、认证、JSON 契约、WebUI 不变（spec D10）
- [ ] `gofmt -l` 无输出；`go test ./...` 全绿（需用户明确授权编译/测试命令）
