# 04: 服务端诚实性 + WebUI 入口收口

**What to build:** 消除管理 API 的"200 但无副作用"端点，并同步收口 WebUI 上对应的成功路径。端到端行为：用户在 WebUI 点"导出配置/导入配置/恢复配置/CCS 导入"时不再看到成功提示；HTTP 调用这些端点返回 501 `not_implemented`。

**当前假行为清单**：

| 端点 | 当前行为 | 目标 |
|---|---|---|
| `settings_handlers.go:handleConfigExportStub` | `200 {ok:true, path:"/tmp/export.json"}`（该文件不存在） | **501** `{error:{type:"not_implemented"}}` |
| `handleConfigImportStub` | `200 {ok:true, message:"imported"}` | **501** |
| `handleConfigRestoreStub` | `200 {ok:true, backup:"/tmp/backup.json"}` | **501** |
| `handleBackups` | `200 []`（无任何真实备份实现） | **501** |
| `handleCcsImport`（POST） | `200 {ok:true, imported:0}` | **501** |
| `handleInit` | `200 {messages:["init ok"]}` | **501**（spec D3：语义不明，不编造行为） |
| `handleCcsProviders`（GET） | `200 {providers:[]}` | **保留 200**——空集合是真值，未声称副作用 |
| `handlePresets`（GET） | **勘误：并非恒空**，返回 4 条真实静态预设 | **保持 200 且保留目录**——这是真值，不是假成功 |

**实现要点**：

- 判据核心（spec 已核实事实第 6 条）：只有**声称发生了副作用**的响应才算假成功。GET 返回空集合保留 200，不要顺手改
- **前后端同批**：后端改 501 的同时，WebUI 必须隐藏/禁用对应入口，否则用户仍看到"点了还成功"。涉及 `webui/src/components/BackupsPanel.tsx`、config export/import/restore 入口、CCS 导入按钮，以及 `webui/src/apiSchema.ts` / `api.ts` 的对应解码
- **必须同步更新的既有测试**：`internal/server/tui_release_test.go:163` 断言 `POST /api/ccswitch/import` 返回 **200**，改 501 后必须更新为期望 501；同文件 :149 的 GET providers 断言保持 200

**Blocked by:** None — can start immediately（与 03 同族，可并行）

**Status:** ready-for-agent

- [ ] export/import/restore、`/api/backups`、ccs POST import、`handleInit` 全部返回 501 且带 `not_implemented` 标识
- [ ] GET 类空集合端点保持 200——`handleCcsProviders` 返回空数组是真值；`handlePresets` **返回真实目录**（04 写入时勘误：它不恒空），二者都不得为"看起来非空"而改动
- [ ] WebUI 不再展示上述端点的成功路径（入口隐藏或禁用并说明原因）
- [ ] `tui_release_test.go` 的 ccs import 断言更新为 501，并新增"声称副作用的端点不得返回 200"的断言
- [ ] 前端契约文件（`apiSchema.ts`/`api.ts`）与新响应一致
- [ ] 测试：每个改为 501 的端点至少一条断言；不存在"200 但无副作用"的端点覆盖
- [ ] `go build ./...` 通过；`go test ./... -count=1` 全绿；`gofmt -l` 无输出
- [ ] 不实现 config 备份/导出/恢复与 CCS 导入的真实逻辑（另立 spec）
