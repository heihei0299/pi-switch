# 01: 保存失败不再静默吞掉（A10）

**What to build:** 配置写盘失败必须让调用方看得见。端到端行为：磁盘满或权限错误时，管理 API 返回 5xx 而不是 `200 {"ok":true}`，CLI 退出码非零且不打印成功文案。

**Blocked by:** None — can start immediately

**Status:** resolved (2026-09-11)

- [x] 六个服务端调用点的 `_ = saveConfig(cfg)` 改为检查错误并返回 500
- [x] CLI 两处 `_ = saveConfigFile` 改为失败时返回非零 + 说明，且不打印 "Switched to"/"Deleted"
- [x] 全仓 `_ = saveConfig` / `_ = saveConfigFile` 归零
- [x] 测试：写盘失败注入下每个受影响端点断言 5xx 且 body 不含 `ok`；CLI 断言非零 + stdout 无成功文案 + stderr 有原因
- [x] 正向对照：同端点/同命令在可写路径上仍返回 200 / 退出 0，证明上面的失败可归因于写盘
- [x] `go build ./...` 通过；`go test ./... -count=1` 全绿；`gofmt -l` 无输出

## 实施记录

- `internal/server/settings_handlers.go`：`handlePutSettings` 写盘失败 → `500 {"error":"failed to save settings: …"}`
- `internal/server/profile_handlers.go`：`handleDeleteProfile`、`handleDuplicateProfile`、`handleFetchModelsForChannel`、`handlePutModels`、`handlePutExpose` 五处 → `500 {"error":"failed to save config: …"}`
- `cmd/pi-switch/main.go`：`provider use`/`provider delete` 写盘失败 → stderr 说明 + 非零退出；顺带把 `handleProvider` 由"直接 `os.Exit`"改为返回退出码（与 `handlePackage` 一致），使其可在进程内测试
- 测试：`internal/server/save_failure_test.go`（5 个端点 + 正向对照）、`cmd/pi-switch/main_test.go` 新增 `TestHandleProvider_ReportsSaveFailure` 与 `_SucceedsOnWritableConfig`
- 失败注入方式：配置文件**可读但所在目录不可写**（`chmod 0500`）——读取能过路由、写入在创建临时文件时失败；根用户下跳过（权限约束不成立），并用 `t.Cleanup` 恢复权限
- **红检**：临时恢复 `handlePutSettings` 的吞错后，断言以 `put settings = 200 ({"ok":true}), want 500` 失败，证明测试能打到该回归

## 验证证据

- `go build ./...` 通过；`gofmt -l` 无输出；`go test ./... -count=1` 16 包全绿
- 全仓 `_ = saveConfig` 已归零（`grep` 无命中）

## 未纳入本项

- 写盘失败时的**恢复机制**（备份/重试/只读兜底）不属本项：本项只让失败可见。
- `POST /api/proxy/start` 等"200 但无工作"端点见 `trust-boundary-and-honesty` 的遗留项。
