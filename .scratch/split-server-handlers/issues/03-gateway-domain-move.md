# 03: gateway 域 handler 迁出 kernel

**What to build:** 把网关管理域的 handler 与 helper 从 `internal/server/server.go` 机械搬进 `internal/server/gateway_handlers.go`，网关行为零变化。端到端行为：读取网关配置、网关请求预览（GET 与 POST 两种形态）、预览渲染、网关计划构建与模型增强、写入网关、健康检查、启动与发布全部照旧工作。

**归入本票的符号**（spec D2 映射表，gateway 行）：

- `handleGetGateway`、`gatewayPreviewRequest`（类型）、`handleGatewayPreview`、`handleGatewayPreviewPost`、`serveGatewayPreview`
- `buildGatewayPlan`、`enrichProposedModels`
- `handlePutGateway`、`handleGatewayHealth`、`handleGatewayStart`、`handleGatewayPublish`

本票是 spec D8「commit 1 纯文件移动」的分片之一，与其他域分片共用同一个移动提交（见 07）。

**Blocked by:** 01: proxy 域 handler 迁出 kernel

**Status:** resolved (2026-09-11)

- [x] 上述符号与 `gatewayPreviewRequest` 类型全部位于 `gateway_handlers.go`，保留原 `internal/server` package，不新增子包、不导出符号（spec D1）
- [x] `server.go` 不再声明上述符号；被 profile 与 gateway 共用的 `resolveModelsDevProvider` 留在 kernel（spec D3）
- [x] 零逻辑改动：函数体、签名、注释逐字不变
- [x] 测试文件零改动；`retry.go`、`outbound.go`、`legacy_log.go` 不动（spec D2/D4）
- [x] 路由注册、认证、JSON 契约、WebUI 不变（spec D10）
- [x] `gofmt -l` 无输出；`go test ./...` 全绿（16 个包；`internal/server` 6.96s）

## 实施记录

- 提交：`b7abcad`
- 交付：`internal/server/gateway_handlers.go`（新增 / 10 个符号，含 `gatewayPreviewRequest` 类型）、`internal/server/server.go`（1697 → 1447 行 / 65 → 55 个符号）
- 随搬迁移出 kernel 的 import：`catalog`（首个被迁空其用途的 import）
- 纯移动证据：函数级逐字节比对 **65/65 IDENT，0 CHANGED / 0 LOST / 0 DUP**
- 验收命令：`gofmt -l internal/server/` 无输出；`go test ./...` 全绿
- **过程记录（重要，03-06 已据此换工具）**：本票最初仍按行号区间切割，再次切错（两个文件都语法错误）。根因是**行号定位**：票 01/02 的搬迁已让行号整体偏移，我读取时得到的位置与实际不符；行号失效不报错，只静默放错位置。
  已改为 `.scratch/split-server-handlers/domove.py`：**按符号名定位**（`cut` 子命令找不到符号即报错退出，不写文件；`verify` 子命令做函数级逐字节比对）。本票即用该工具完成，一次通过。后续 04-06 沿用
