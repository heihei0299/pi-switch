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

**Status:** ready-for-agent

- [ ] `package add/show/delete` 真实生效（写入/查询/删除可在后续 list 中观察到）
- [ ] `handleDoctor` 报告真实探测结果，探测失败时非零退出且 stdout 不含 `ok`
- [ ] `handleStatsCLI` 返回真实统计或明确未实现 + 非零退出
- [ ] 未实现的命令：stderr 说明 + 非零退出，stdout 不含 `ok`
- [ ] 抽出的核心函数被 handler 与新 CLI 共用（不出现同一逻辑两份实现）
- [ ] 测试：每个假成功命令至少一条断言"未实现时退出码非零且 stdout 不含 ok"；已接线命令有一条断言真实副作用
- [ ] `go build ./...` 通过；`go test ./... -count=1` 全绿；`gofmt -l` 无输出
- [ ] 不处理 `_ = saveConfig` 的静默吞错（A10 属 P1 另一族，不在本 spec）
