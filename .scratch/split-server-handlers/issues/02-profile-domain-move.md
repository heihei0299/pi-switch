# 02: profile 域 handler 迁出 kernel

**What to build:** 把供应商/配置文件管理与模型目录链路的 handler 与 helper 从 `internal/server/server.go` 机械搬进 `internal/server/profile_handlers.go`，供应商管理行为零变化。端到端行为：WebUI 读写配置、profiles 的增删改/复制/连通性测试、从渠道拉取模型列表、模型目录增强、单价与曝光/伪装开关、Credits、预置、doctor、CCS 导入导出全部照旧工作。

**归入本票的符号**（spec D2 映射表，profile 行）：

- 配置读写：`handleGetConfig`、`handlePutConfig`、`handleGetState`
- profiles CRUD 与测试：profiles CRUD/duplicate/test、`validateProfileResponsesMode`、`truncateForTest`
- fetch-models 链路：`handleFetchModelsForChannel`、`fetchUpstreamUsage`、`fetchUpstreamIDs`、`handleFetchModels`、`enrichModelsWithCatalog`
- 模型与渠道设置：`handlePutModels`、`handlePutExpose`、`handlePutSpoof`、`handleGetCredits`、`handlePresets`、`handlePresetDetail`、`handleDoctor`
- 校验与渠道辅助：`validateResponsesMode`、`isValidChannelName`、`ensureMutationChannel`、`channelIndex`、`validateProviderProfile`、`handleValidate`
- CCS：`handleCcsProviders`、`handleCcsImport`

本票是 spec D8「commit 1 纯文件移动」的分片之一，与其他域分片共用同一个移动提交（见 07）。映射表基于 commit `46d8153`；若发现清单漂移，以「域归属 + kernel 规则」为准，不扩大范围。

**Blocked by:** 01: proxy 域 handler 迁出 kernel

**Status:** resolved (commit 见下, 2026-09-11)

- [x] 上述符号全部位于 `profile_handlers.go`，保留原 `internal/server` package，不新增子包、不导出符号（spec D1）
- [x] `server.go` 不再声明上述符号；被 2+ 域调用的 helper（如 `resolveModelsDevProvider`）留在 kernel（spec D3）
- [x] 零逻辑改动：函数体、签名、注释逐字不变
- [x] 测试文件零改动；`retry.go`、`outbound.go`、`legacy_log.go` 不动（spec D2/D4）
- [x] 路由注册、认证、JSON 契约、WebUI 不变（spec D10）
- [x] `gofmt -l` 无输出；`go test ./...` 全绿（16 个包；`internal/server` 6.99s）
- [x] 不触碰 `configPath` / `saveConfig` 的收敛（08 的范围）

## 实施记录

- 交付：`internal/server/profile_handlers.go`（新增 1093 行 / 33 个符号）、`internal/server/server.go`（2774 → 1697 行 / 98 → 65 个符号）
- 搬迁来源为两段非连续区间：主体 `handleGetConfig`..`handleValidate` 连续段，加 `handleCcsProviders`/`handleCcsImport`（这两者被 `handlePackageToggle` 与 `handleInit` 夹在 package 域与 settings 域之间；spec D2 把 CCS 归 profile 域，故一并迁出）
- 随搬迁移出 kernel 的 import：`io`、`net/http`（profile 块是它们最后的用户）
- 纯移动证据：函数级逐字节比对 **98/98 IDENT，0 CHANGED / 0 LOST / 0 DUP**；符号总数守恒 65 + 33 + 20 = 118
- 验收命令：`gofmt -l internal/server/` 无输出；`go test ./...` 全绿
- **过程记录（供后续票警惕）**：本次搬迁最初用按行号切割的脚本执行，切点错位导致 `handleValidate` 的函数尾被复制到两个文件、`server.go` 出现孤立残尾，两文件均有语法错误。`gofmt -e` 检出后以定点编辑修复，校验器确认无内容丢失（0 LOST）。**后续域搬迁（03-06）改用 `edit` 工具按字面锚点切割，不使用行号脚本。**
- 校验器缺陷修正：原比对脚本以「列 0 的 `}`」作函数终止判定，遇到一行式函数 `func f() { ... }` 会卡死并吞掉其后所有符号（曾使票 01 的校验只覆盖 115/118）。已改为按「下一个 `^func` 声明」定界并修正签名跨行判定；修正后回测票 01 得 **118/118 IDENT**
