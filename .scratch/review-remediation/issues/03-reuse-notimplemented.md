# 03: 复用 kernel `notImplemented`（Standards 硬性发现）

Status: resolved (2026-09-11)

**问题**：`settings_handlers.go:130` 手写了 `server.go:164` 的 `notImplemented(what)` 已产出的
`{error:{type:not_implemented,message:…}}` 信封，而该 helper 已有 7 个调用点；注释还声称与该批形状一致。
违反 AGENTS.md 实现约束 §2/§6 与 architecture.md §3。

**修**：改用 helper。注意 helper 的 message 是 `what + " is not implemented"` 固定后缀，选词要让整句通顺，
不能照抄 reviewer 示例（会把可操作提示变成别扭句子）。

**验收**
- [ ] 该端点走 helper，仓库内不再有第二处手写 not_implemented 信封
- [ ] 501 的 `error.type` 仍是 `not_implemented`，message 仍说明需 daemon/host/port
- [ ] `stub_honesty_test.go` 与票 01 的测试保持绿
