# 04: package 域 handler 迁出 kernel

**What to build:** 把 pi 包管理域的 handler 与 DB helper 从 `internal/server/server.go` 机械搬进 `internal/server/package_handlers.go`，包管理行为零变化。端到端行为：列出已安装 pi 包、从 pi 配置导入包、包相关 DB 路径解析与建表迁移、`pi agent` 设置路径解析全部照旧工作。

**归入本票的符号**（spec D2 映射表，package 行）：

- DB 与路径：`piSwitchDBPath`、`openPiSwitchDB`、`ensurePiPackageColumn`、`piAgentSettingsPath`
- 解析与业务：`parsePackageSpec`、`ListInstalledPackages`、`ImportPiPackages`、`boolInt`
- 全部 `handlePackage*` 入口（当前 6 个）

本票是 spec D8「commit 1 纯文件移动」的分片之一，与其他域分片共用同一个移动提交（见 07）。

**Blocked by:** 01: proxy 域 handler 迁出 kernel

**Status:** resolved (2026-09-11)

- [x] 上述符号全部位于 `package_handlers.go`，保留原 `internal/server` package，不新增子包、不导出符号（spec D1）
- [x] `server.go` 不再声明上述符号，但 6 个 `handlePackage*` 的路由注册仍留在 kernel
- [x] 零逻辑改动：函数体、签名、注释逐字不变
- [x] 测试文件零改动；`retry.go`、`outbound.go`、`legacy_log.go` 不动（spec D2/D4）
- [x] 路由注册、认证、JSON 契约、WebUI 不变（spec D10）
- [x] `gofmt -l` 无输出；`go test ./...` 全绿（16 个包；`internal/server` 7.25s）
- [x] 不做 L3 跨包迁移（把 package 域抽到 `internal/piagent` 属 out of scope，另立 spec）

## 实施记录

- 交付：`internal/server/package_handlers.go`（新增 394 行 / 14 个符号）、`internal/server/server.go`（1446 → 1065 行 / 55 → 41 个符号）
- 14 个符号一次切出（8 个 DB/路径/解析 helper + 6 个 `handlePackage*`），搬迁块内被 `handlePackageToggle`/`handleInit` 与 settings 域分隔的情形已由按名定位工具正确处理
- import 转移：`net/url` 从 server.go 移入 package_handlers.go（`ListInstalledPackages` 的 `url.PathUnescape`）；`piagent` 随 `ImportPiPackages` 迁出
- 纯移动证据：函数级逐字节比对 **55/55 IDENT，0 CHANGED / 0 LOST / 0 DUP**
- 验收命令：`go build ./...` 通过；`go test ./...` 全绿；`gofmt -l` 无输出
- **过程记录（方法修正）**：本次先用函数级校验通过（55/55 IDENT），但 `go test` 报 build failed——`net/url` 的归属被我漏判。**结论：函数级校验只证明"代码没被改写"，不证明"符号可解析"；每张搬迁票必须跑 `go build`，且 import 归属要靠「搬迁块用到的全部包限定符」清单机械枚举，不能靠人工扫读。** 已把该清单加为 `domove.py cut` 的固定输出
