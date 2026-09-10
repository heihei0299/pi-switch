# 02: 标注 retry.go 为休眠原语（A4）

**What to build:** 让读者第一眼就知道 `retry.go` 的重试/故障转移引擎当前不生效，同时不误导成"整个文件都是死代码"。端到端行为：新读者不会再误判 failover 可用，也不会顺手删掉它。

**Blocked by:** None — can start immediately

**Status:** resolved (2026-09-11)

- [x] `retry.go` 文件头说明"当前休眠、代理路径为单候选直通、复用前提见 spec"
- [x] 说明区分"引擎休眠"与"校验仍在用"（后者仍被 profile/settings handler 调用）
- [x] `docs/architecture.md` 补同一事实
- [x] 说明中给出可查的出处（`remove-failover-chain/spec.md` D1）与"不要指望改动此处影响生产/不要未经确认删除"的告警
- [x] `go build ./...` 通过；`go test ./... -count=1` 全绿；`gofmt -l` 无输出

## 实施记录

实施前核实（这决定了注释怎么写才不误导）：

- **零生产调用**：`expandAttempts`、`admitForRound`、`waitForRound`、`classifyUpstreamError`、`classifyTransportError`、`coolForAttempt`、`isCooling`、`markCooling`、`candidateCooldownKeys`、`coolingHint`、`orderedChannels`、`channelWeight`、`effectiveRequestRetry`、`primaryChannel`、`activeScopedRules` 在非测试代码中均无调用
- **仍在生效**：`validateRetryFields`（`profile_handlers.go` 三处）、`validateSettingsRetry`（`settings_handlers.go`、`profile_handlers.go`）
- 生产路径确实只取单候选：`proxy_handlers.go` 的 `candidates[0]`；`PUT /api/proxy/failover` 返回 410

已加的文件头说明明确列出休眠的调度符号、点明校验函数仍在使用、给出 spec 出处，并写明"不要期望此处改动影响生产，未经确认不要删除"。

## 验证证据

- `go build ./...` 通过；`gofmt -l` 无输出；`go test ./... -count=1` 16 包全绿（注释改动，无行为影响）
