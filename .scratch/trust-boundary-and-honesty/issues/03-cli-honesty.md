# 03: CLI 诚实性——能接线的接线，不能的改非零退出

**What to build:** 消除 CLI 的"假成功"，让 stdout 的 JSON 与实际发生的事一致。端到端行为：`pi-switch package add <spec>` 真的写入包记录；未实现的能力在 stderr 说明并以非零退出，stdout 不再出现 `ok`。

**当前假行为清单**（`cmd/pi-switch/main.go`）：

| 命令 | 当前行为 | 目标 |
|---|---|---|
| `package add` | 打印 `{"ok":true,"id":...}` 不做事 | **接线**到服务端已有真实逻辑（`handlePackageAdd`，spec D2） |
| `package sync` | 打印 `{"ok":true,"message":"sync done"}` | 按真实语义实现或 501 |
| `package show` | 打印 `{"error":"not found"}` | **接线**（`handlePackageGet`/`handlePackagesList`） |
| `package delete` | 打印 `{"ok":true}` | **接线**（`handlePackageDelete`，spec D2） |
| `handleCcs` | 返回空数组 | 空数组是**真值**（确实没有），保留 200 语义；若参数指向真实路径却查不到，需非零退出 |
| `handlePresets` | 恒空 | 同 `handleCcs` 的判据 |
| `handleStatsCLI` | 仅提示用 webui | 查真实数据，或明确"未实现"+非零退出 |
| `handleDoctor` | 永远 `doctor: ok` | **真实探测**（config 可读、DB 可开、端口占用、daemon 状态） |
| `package list`、`package import` | 已真实实现 | 不动（作为同族一致性的参照） |

**实现要点**：

- spec D2：把 `handlePackageAdd`/`handlePackageGet`/`handlePackageDelete` 的**核心逻辑抽成函数**，handler 与新 CLI 共用，不让 CLI 走 HTTP 调管理 API
- 判据核心（spec 已核实事实第 6 条）：只有**声称发生了副作用**的输出才算假成功；返回空集合但没声称做过事，不是假成功，不要为了"看起来不空"去改它

**Blocked by:** None — can start immediately

**Status:** resolved (2026-09-11)

- [x] `package add/show/delete` 真实生效（写入/查询/删除可在后续 list 中观察到）
- [x] `handleDoctor` 报告真实探测结果，探测失败时非零退出且 stdout 不含 `ok`
- [x] `handleStatsCLI` 返回真实统计或明确未实现 + 非零退出
- [x] 未实现的命令：stderr 说明 + 非零退出，stdout 不含 `ok`
- [x] 抽出的核心函数被 handler 与新 CLI 共用（不出现同一逻辑两份实现）
- [x] 测试：每个假成功命令至少一条断言"未实现时退出码非零且 stdout 不含 ok"；已接线命令有一条断言真实副作用
- [x] `go build ./...` 通过；`go test ./... -count=1` 全绿；`gofmt -l` 无输出
- [x] 不处理 `_ = saveConfig` 的静默吞错（A10 属 P1 另一族，不在本 spec）

## 实施记录

### 交付

- `internal/server/package_handlers.go`：抽出 `AddInstalledPackage`、`GetInstalledPackage`、`DeleteInstalledPackage` 与哨兵 `ErrPackageNotFound`；`PiSwitchDBPath` 导出；三个 handler 改为委托，删除原内联实现
- `internal/server/profile_handlers.go`：抽 `ProviderPresets()`，`handlePresets` 改为委托
- `internal/server/server.go`：`isLoopback` → `IsLoopback`（导出，消除 CLI 副本）、`storedWebUIPassword` → `StoredWebUIPassword`、新增 `WebUIPasswordConfigured`
- `cmd/pi-switch/main.go`：`handlePackage`/`handleCcs`/`handlePresets`/`handleStatsCLI`/`handleDoctor`/`handleConfigCLI` 由「直接 `os.Exit`」改为「返回退出码」，dispatch 处 `os.Exit(handler(...))`；help 文本更正；新增 `--disabled` 与 `package import` 说明
- `cmd/pi-switch/main_test.go`：14 个用例（含 stdout/stderr 双流捕获）
- `README.md` / `README_ZH.md`：更正本票触碰过的 CLI 行

### 关键判断（判据核心的应用）

- **`ccs list` 空数组 = 真值，不动**。全仓确无 CCS provider 存储，空集合未声称任何副作用。
- **`presets` 是失真，接线**。服务端 `handlePresets` 一直返回 4 条真实静态预设（旧 Node CLI 的 `preset list` 同样返回真实目录），而旧 Go CLI 打印 `[]` 且 `preset show <id>` 对已存在的 id 也报 not found。已在 spec 与票 04 中**勘误**"presets 恒空"的错误前提。
- **`package sync`/`ccs import`/`stats` → 非零退出**，不做语义猜测；`doctor` 改为真实探测。

### 过程中发现并修掉的真缺陷

1. **`LoadConfigAtPath` 对坏 JSON 静默返回默认配置**（source `"default (bad json)"`），所以靠 `err != nil` 的 doctor 探测**永远查不出损坏配置**；`config validate` 同样因此把损坏文件报成 `config valid`（默认配置含占位 profile，`len(Profiles) > 0` 恒真）。两处均已改为直接校验字节（`configFileProblem`）。
2. **`openPiSwitchDB` 会建目录与建表**——doctor 若用它探测就产生副作用。改为只读 `os.Stat` + `sql.Open`/`Ping`。
3. **doctor 打印的是指针地址而非 PID**：`%v` 作用于 `*uint32` 会输出 `0xc000…`。已改为解引用。
4. **daemon 状态说明被丢弃**：`daemon.Status` 在「进程活着但端口不响应」时返回 `Running=false` 并附原因，旧代码一律打印 "not running"。现已把其 `Message` 一并输出。
5. **`IsLoopback` 一度把 `[::1]:43110` 判为非 loopback**（fail-closed 方向，但属回归）。已修并按多种写法加用例锁定。
6. **`package add` 静默丢弃多余参数**：会按与 README 契约不符的输入持久化记录。已改为拒绝并说明 spec 是单 token。
7. **help 与行为矛盾**：help 宣传 `package sync`、`stats`，而它们现在必定失败；`presets [list]` 的写法此前会走 unknown-id 路径。均已更正/兼容。

### Review 结果（Step ③：两个独立单轴 reviewer 并行）

- **Standards-only**：2 BLOCKING、6 follow-up、6 advisory，**已全部处理**。BLOCKING：`package add` 静默丢参（已拒绝并修 README）、help 与行为矛盾（已修）。follow-up：导出 `IsLoopback`/`WebUIPasswordConfigured` 消除两份重复实现、`isolateCLI` 补 `PI_AGENT_SETTINGS`/`PI_CODING_AGENT_DIR`、删除不可达的 loader-error 分支、断言改 `json.Unmarshal`、`--disabled` 文档化。
- **Spec-only**：AC1/3/4/5/6/8 满足，AC2 PARTIAL → **已修**（见下），AC7 由本代理执行。2 BLOCKING：**① doctor 在 DB 存在时仍打印 `database: … ok`**，导致探测失败时 stdout 含成功 token（测试盲点：原 B10 的临时目录没有 DB）→ 已改为 `readable` 并新增覆盖该分支的用例；**② `presets list` 回归**（help 宣传但实际报 unknown preset）→ 已接受 `list`/`ls`/`-h` 为列表形式。
- **两个 reviewer 都再次报告审查期间工作区未冻结**（文件数 4→8、行数多次变化）。我在这两轮里仍未能做到「编辑与 review 严格串行」，这是流程失误；缓解措施是把最终验证放在最后一次修改之后重跑，并以最终 diff 为准。**后续票必须先冻结再开 review。**

### 验证证据

- `go build ./...` 通过；`go vet ./internal/server/ ./cmd/...` 无输出；`gofmt -l internal/ cmd/` 无输出；`go test ./... -count=1` **16 包全绿**
- 真实运行（临时目录隔离，结束已清理）：`package add` → `list` 可见 → `delete` → `list` 空 → 重复 `delete` 退出 1；`package add npm:x Name 1.2.3` 退出 1 并给出可操作提示；`package sync`/`ccs import`/`stats` 退出 2 且 stdout 空；`ccs list` 退出 0 且 `{"providers":[]}`；`presets`/`presets list`/`presets show openai` 均退出 0 并输出真实目录；`doctor` 健康退 0、坏 config 退 1 且 stdout 无成功 token、DB 异常退 1；`config validate` 对坏文件退 1

### 未纳入本票的后续项

- **README CLI 段落仍有本票之外的失真**：`package toggle`、`config backups`、`config export/import`、`import ccswitch` 实测均返回 1（未实现或不存在）。本票只改自己触碰过的行，其余留待文档收口票。
- `package show <id>` 在 `delete` 后仍返回墓碑记录（`installed:false`、退出 0）：这是有意保留——核心函数的契约是"该 id 是否存在任何行"，墓碑对排查有用，已在文档注释中写明。
- `GetInstalledPackage` 现在把非 `ErrNoRows` 错误映射为 500（HEAD 是无差别 404），属诚实性正向变化但改变了 HTTP 契约；WebUI 调用点为 `webui/src/api.ts:135`，留待 04 一并核对。
