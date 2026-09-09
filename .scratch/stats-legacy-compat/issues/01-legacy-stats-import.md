# 01 - 旧请求日志导入与双写恢复

Status: resolved

## 背景

见 `../spec.md`。Go 版统计只读 SQLite，存量 `requests.log`（23394 行）不可见，且新请求不再追加日志。

## 工作内容

1. 启动时幂等导入请求日志历史行到 SQLite（自然键去重，坏行跳过计数，不阻断启动）。
2. 新请求恢复双写：SQLite + 请求日志（旧 camelCase 字段形状，缺消费省略 `costTotal`）。
3. 字段映射按 spec（`ok`→成功标志，token 四件套，`costTotal`→消费缺失记 unknown，无延迟记 NULL，对话标识直存）。

## 验收标准

- 给定含旧格式行（含缺 `costTotal`、缺 `conversationName`、坏行各一）的 fixture 日志，启动后统计接口 `totalRequests` 包含全部合法旧行，`costUnknown` 至少为缺消费行数，坏行被跳过。
- 重启后再次导入，统计总数不变（幂等）。
- 经 mock 上游完成一次新请求后，SQLite 新增一行且请求日志尾部新增一行旧形状 JSON。
- 统计、对话、明细、导出四个接口均能看到旧行数据（口径不变）。
- 全量 Go 测试通过；WebUI 统计相关测试通过（`NODE_ENV=test`）。

## 非目标


## 实施总结
- 提交：`e1fb029` — `feat(stats): import legacy requests.log history and dual-write new requests`
- 实现的 seams：Go 二进制外部行为（`GET /api/stats`、`GET /api/stats/conversations`、`GET /api/stats/conversations/:id/requests`、`GET /api/logs/export` + `requests.log`/`requests.db` 落盘）
- 验收标准：
  - [x] fixture 旧行（缺 `costTotal`、缺 `conversationName`、坏行）→ `totalRequests` 含 2 合法行，`costUnknown` 为 1，坏行跳过（`TestLegacyLog_ImportedIntoStats`）
  - [x] 重复导入总数不变（`TestLegacyLog_RequeryDoesNotDuplicate`、`TestLegacyLog_AppendedLinesImportedOnce`；实现为自然键去重 + 文件指纹，重启走同一路径 `ImportLegacyOnStartup`）
  - [x] mock 上游新请求 → SQLite +1 且日志尾部 +1 旧形状行（含 `promptTokens`/`costTotal`/`status`）（`TestLegacyLog_NewRequestDualWritesLog`）
  - [x] 四个接口均返回旧行（`TestLegacyLog_ImportedIntoStats` + `TestLegacyLog_VisibleInConversationsAndExport`）
  - [x] 全量 Go 测试通过（`go test ./...` 全绿，含 server 包）；WebUI 26 文件 219 测试通过（`NODE_ENV=test npm --prefix webui run test`）
- 测试结果：新增 5 个 seam 测试全绿；全量套件全绿
- typecheck：`go build ./...` 通过
- 文档对齐：无需更新（README 已描述请求日志追加与零迁移，实现补齐后与文档一致）
- 遗留 / 后续建议：code-review Standards 轴的 4 类命名/结构判断（`logRequest` 长参数列、`ensureLegacyImported` 四处调用、`legacyStr/legacyInt` 形状重复）为不重构判断——参数列系既有形状、调用点已收敛为单行 helper；旧日志中无 token 字段的极早行（2 行）导入为 0 token 行，仅影响 `costUnknown` 计数 +2，可接受
- 不改统计聚合口径与前端展示逻辑；不重算旧行消费；不动旧日志文件内容。
