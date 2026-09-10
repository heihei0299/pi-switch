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

**Status:** resolved (2026-09-11)

- [x] export/import/restore、`/api/backups`、ccs POST import、`handleInit` 全部返回 501 且带 `not_implemented` 标识
- [x] GET 类空集合端点保持 200——`handleCcsProviders` 返回空数组是真值；`handlePresets` **返回真实目录**（04 写入时勘误：它不恒空），二者都不得为"看起来非空"而改动
- [x] WebUI 不再展示上述端点的成功路径（入口隐藏或禁用并说明原因）
- [x] `tui_release_test.go` 的 ccs import 断言更新为 501，并新增"声称副作用的端点不得返回 200"的断言
- [x] 前端契约文件（`apiSchema.ts`/`api.ts`）与新响应一致
- [x] 测试：每个改为 501 的端点至少一条断言；不存在"200 但无副作用"的端点覆盖
- [x] `go build ./...` 通过；`go test ./... -count=1` 全绿；`gofmt -l` 无输出
- [x] 不实现 config 备份/导出/恢复与 CCS 导入的真实逻辑（另立 spec）

## 实施记录

### 交付

- `internal/server/server.go`：新增 kernel helper `notImplemented(what) gin.H`（统一 501 body）。**放在 kernel 是必须的**：它被 settings 与 profile 两个域调用，而 `docs/architecture.md` 与 split spec D3 规定"被 2+ 域调用的 helper 必须留在 kernel、域文件之间不得互调内部 helper"（Spec/Standards reviewer 均指出我最初把它放在 `settings_handlers.go` 违反此规则，已修）
- `internal/server/settings_handlers.go`：`handleConfigExportStub`/`handleConfigImportStub`/`handleConfigRestoreStub`/`handleBackups`/`handleInit` → 501
- `internal/server/profile_handlers.go`：`handleCcsImport` → 501；`handleCcsProviders` 与 `handlePresets` 保持 200
- 前端：`BackupsPanel.tsx` 改为说明卡片（无任何可交互控件）；**删除整个 "Import from cc-switch" 弹窗与入口按钮**；`App.tsx` 的 "Initialize config" 按钮（直连 `api.init()`）改为说明；删除 `api.ts` 7 个方法、`apiSchema.ts` 7 个解码器、`types.ts` 两个类型、`e2e/layout.spec.ts` 的死 mock、`i18n.tsx` 15 个死键
- 测试：新增 `internal/server/stub_honesty_test.go`、`webui/src/components/BackupsPanel.test.tsx`、`ProfilesPanel.test.tsx` 的 cc-switch 缺失守卫；`tui_release_test.go` 的 200 断言改为复用 `assertNotImplemented`

### Review 结果（Step ③：两个独立单轴 reviewer 并行，工作区全程冻结）

- **Spec-only**：标准 1–6、8 SATISFIED，**无 blocking**；路由交叉核对确认 8 个端点全部注册且行为与目标列一致；勘误后的前提成立（预设仍是真实 4 条目录，未被清空）
- **Standards-only**：**1 BLOCKING**（`notImplemented` 放在设置域却被 profile 域调用，违反 D3）→ 已移入 kernel；follow-up：测试断言三处重复（已收敛为一处共享断言）、弱重复测试（已删）、恒真的预设断言（已删并把测试名改为其真正覆盖的内容）、`decodeStringArray`/`CcsImportResult`/i18n 死键（已清理）、ProfilesPanel 缺缺失守卫（已补）
- 我按要求把断言从"按钮文案"改为"面板不含任何可交互控件"，避免改名即可绕过

### 验证证据

- `go build ./...` 通过；`go vet ./...` 无输出；`gofmt -l internal/ cmd/` 无输出；`go test ./... -count=1` **16 包全绿**
- WebUI：`tsc --noEmit` 无错；`vitest run` **268 测试 / 33 文件全绿**
- 真实运行（临时端口，已清理）：6 个端点全部 **501**、`/api/ccswitch/providers` 与 `/api/presets` 保持 **200**、`/api/presets` 仍返回 **4 条**目录、501 body 为 `{"error":{"message":"config backups is not implemented","type":"not_implemented"}}`
- **未能验证**：Playwright e2e（浏览器未安装，`browserType.launch: Executable doesn't exist`）。安装需下载数百 MB，未擅自执行。我对 `e2e/layout.spec.ts` 的改动仅为删除一行已失效 mock，风险低，但如实记为未实跑

### 已知取舍与后续项

- **有意的能力放弃**：随方法一起删掉的 `api.ccsProviders` 使 WebUI 不能再列出 cc-switch 的 provider（GET 端点本身仍诚实地返回 200）。CLI 侧 `pi-switch ccs list` 仍可用。这是有意的产品取舍，非隐藏回归
- **out-of-table 的"200 但无工作"端点仍然存在**：`settings_handlers.go` 的 `POST /api/proxy/start` 非 daemon 分支返回 `running:true, message:"proxy started (stub)"`，`gateway_handlers.go` 的 start/health 返回恒定成功 payload。它们让 spec 的全局判据"不存在返回 200 但无副作用的端点"在**全仓范围**内仍不成立，需单独立票
- **值得记录的事实**：`src/sync.js`（及 `extensions/index.ts` 的接线）是**真实的加密导出/导入实现**，作用在同一个 `~/.pi-switch/config.json` 上——"没有实现"只在服务端范围内成立。将来做真实备份/导出 spec 时应以它为参考
- `req()`（`webui/src/api.ts`）只能解出字符串型 `error`，对象型会退化为 `statusText`；本轮 501 已无 UI 调用者，故属潜在问题，待新调用者出现时一并处理
