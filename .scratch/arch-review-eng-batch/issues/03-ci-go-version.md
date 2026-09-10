# 03: CI Go 版本对齐 go.mod（A11）

**What to build:** CI 不再隐式下载工具链，Go 版本以 `go.mod` 为单一事实来源。

**Blocked by:** None — can start immediately

**Status:** resolved (2026-09-11)

- [x] `ci.yml` 三个 job 的 `go-version: "1.23"` 改为 `go-version-file: go.mod`
- [x] 版本漂移由工具消除（而非靠人工同步常量）
- [x] 不引入新的 GOTOOLCHAIN 策略分支
- [x] `go test ./... -count=1` 全绿（本项不改 Go 代码）

## 实施记录

- 三处（原 `ci.yml:36/150/193`）改为 `go-version-file: go.mod`：setup-go 会直接读取 `go 1.24.2`，此前的 `"1.23"` 低于它，依赖 `GOTOOLCHAIN=auto` 在 CI 内隐式下载工具链
- 选择 `go-version-file` 而不是把 `"1.23"` 改成 `"1.24.2"`：后者仍需人工随 `go.mod` 同步，正是本项要消除的漂移来源

## 验证证据

- `grep -n "go-version" .github/workflows/ci.yml` → 三处均为 `go-version-file: go.mod`，已无硬编码版本
- `go.mod` 为 `go 1.24.2`；本项不改 Go 代码，本地全量测试全绿

## 未纳入本项

- 无法在本机验证 CI 实跑（需推送到 GitHub）；判据"CI 不触发工具链下载"以配置层面消除硬编码版本来满足。
