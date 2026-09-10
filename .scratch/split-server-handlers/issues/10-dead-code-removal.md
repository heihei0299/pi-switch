# 10: 零引用死代码删除

**What to build:** 删除已确认全仓（含测试）零引用的死代码，减少误读与误改。端到端行为：无用户可见行为变化；读者与 agent 在 `internal/server` 内不再遇到「看起来可用、实际没人调用」的符号。

**删除清单**（spec D7）：

- `contains`（数组包含 helper，零调用者）
- `modelsDevCatalog`（模型目录映射变量，零调用者）

**明确保留**：`dump400` 同样零调用者，但它与 `/api/dumps` 端点是成对的生产者/消费者设计，删除生产者会让链路不完整，因此保留原样；作为独立清理候选记录在 spec 的 Further Notes 中，本票不动。`/api/dumps` 端点本身也不删（spec Out of Scope）。

本票与文件搬迁零交集，可立即并行执行；在 01–07 的移动提交中或其后落地均可，但不得混进纯移动提交的 diff。

**Blocked by:** None — can start immediately

**Status:** resolved (2026-09-11)

- [x] `contains`、`modelsDevCatalog` 已删除，且删除前 grep 确认全仓（含 `*_test.go`）零引用
- [x] `dump400` 与 `/api/dumps` 端点保留原样，未顺手删除
- [x] 未扩大删除范围：本票之外不再删除任何符号（spec Further Notes 的「不扩大删除范围」约束）
- [x] `gofmt -l` 无输出；`go test ./...` 全绿

## 实施记录

- 提交：`5fb5c76（与 07 合并提交）`
- `modelsDevCatalog` 位于 `profile_handlers.go`（票 02 随 profile 域迁出后落点），`contains` 位于 `server.go`（票 10 之前一直留在 kernel）
- 删除前独立确认零引用：`contains` 全仓仅 1 处命中即其自身定义（其余命中是英文 "contains" 出现在注释/测试串里）；`modelsDevCatalog` 全仓仅 1 处命中即其自身定义
- `server.go`：544 → 535 行；`profile_handlers.go` −9 行
- 验收命令：`go build ./...` 通过；`go test ./...` 全绿；`gofmt -l` 无输出
- **审计附带发现（未删，记录供后续清理票）**：`effectiveConversationID` 全仓（含测试）零调用者。spec D7 仅授权删 `contains` 与 `modelsDevCatalog`，且 Further Notes 明确「不在 spec 外扩大删除范围」，故保留原样并记入 07 的遗留候选
