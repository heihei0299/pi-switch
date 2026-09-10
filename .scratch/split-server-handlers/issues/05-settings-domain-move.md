# 05: settings 域 handler 迁出 kernel

**What to build:** 把设置与运行时状态域的 handler 从 `internal/server/server.go` 机械搬进 `internal/server/settings_handlers.go`，设置类行为零变化。端到端行为：备份列表、代理运行状态查询、WebUI 信息、构建信息、初始化、代理起停、failover 配置、settings 读写、config export/import/restore 三个 stub 全部照旧工作。

**归入本票的符号**（spec D2 映射表，settings 行）：

- `handleBackups`、`handleProxyStatus`、`handleWebUIInfo`、`handleBuildInfo`、`handleInit`
- `handleProxyStart`、`handleProxyStop`、`handlePutFailover`
- `handleGetSettings`、`handlePutSettings`
- config export / import / restore 三个 stub

本票是 spec D8「commit 1 纯文件移动」的分片之一，与其他域分片共用同一个移动提交（见 07）。注意：构建信息 handler 迁走，但 `embed` 变量与 build info 数据本身属 kernel，不随之移动。

**Blocked by:** 01: proxy 域 handler 迁出 kernel

**Status:** resolved (2026-09-11)

- [x] 上述符号全部位于 `settings_handlers.go`，保留原 `internal/server` package，不新增子包、不导出符号（spec D1）
- [x] `server.go` 不再声明上述符号；embed 变量与构建信息数据仍在 kernel，路由注册仍在 kernel
- [x] 零逻辑改动：函数体、签名、注释逐字不变
- [x] 测试文件零改动；`retry.go`、`outbound.go`、`legacy_log.go` 不动（spec D2/D4）
- [x] 路由注册、认证、JSON 契约、WebUI 不变（spec D10）
- [x] `gofmt -l` 无输出；`go test ./...` 全绿（16 个包；`internal/server` 7.01s）

## 实施记录

- 交付：`internal/server/settings_handlers.go`（新增 161 行 / 13 个符号）、`internal/server/server.go`（1065 → 914 行 / 41 → 28 个符号）
- 13 个符号一次切出：`handleBackups`、`handleProxyStatus`、`handleWebUIInfo`、`handleBuildInfo`、`handleInit`、`handleProxyStart/Stop`、`handlePutFailover`、`handleGet/PutSettings`、三个 config export/import/restore stub
- `handleBuildInfo` 迁出，但 `Version`/`BuildTime`/`BuildCommit`/`BuildTarget`/`BuildDirty` 与 `embed` 变量仍留 kernel（handler 读取它们，同 package 可见）
- 随搬迁移出 kernel 的 import：`daemon`（仅 `handleProxyStatus` 的 `daemon.Status` 使用）
- 纯移动证据：函数级逐字节比对 **41/41 IDENT，0 CHANGED / 0 LOST / 0 DUP**
- 验收命令：`go build ./...` 通过；`go test ./...` 全绿；`gofmt -l` 无输出
