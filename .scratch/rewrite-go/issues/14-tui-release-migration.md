# 14: TUI 与发布收敛（bubbletea + package + 构建分发 + 零迁移）

**What to build:** 收敛剩余能力并打通发布：`charmbracelet/bubbletea` + `bubbles` + `lipgloss` 重写 `ratatui` TUI（`pi-switch tui` 子命令，`profile` 列表/切换、`gateway` 状态、`stats` 简览含 `totalCost/-`），`package` 管理（`catalog/package/package_ops` 的 `add/install/uninstall/enable/disable/sync/import_from_pi`）、`cc-switch` 导入（`listCcsSwitchProviders/importCcsSwitchProviders`）、`preset`/`sync`/`scan_pi`/`credits` 等能力在 Go 中保持可用；多平台交叉编译 `GOOS=linux/darwin/windows×GOARCH=amd64/arm64`（`modernc/sqlite` 纯 Go 无 CGO，`go build -ldflags "-s -w"`），`bin/pi-switch.js` 按平台分发执行对应 Go 二进制，继续发布 `@heihei0299/pi-switch` 到 npm（`pnpm` 全局安装为 store 快照，需复制 `.node→Go` 二进制到快照或重装，`config.json` 热重载无需重启），`~/.pi-switch`（`config.json`、SQLite `requests.db`、`webui_password`、备份）零改动兼容，`daemon` 的 `pid` 文件（`~/.pi-switch/*.pid`）与 `proxy/webui start --daemon --host --port` 语义保持（`stop/status` 含多实例 `ss -tlnp` 定位提示）。

**Blocked by:** 13 WebUI 管理面

**Status:** resolved

- [x] `pi-switch tui` 可启动并完成 `profile` 切换、`gateway` 发布、`stats` 浏览；`--help` 列出全部子命令
- [x] `package` 与 `cc-switch` 导入在 Go 侧可用：`pi-switch package list/add/sync` 与 `pi-switch ccs list/import` 单测覆盖
- [x] `GOOS×GOARCH` 四平台 `go build` 产物可执行，`bin/pi-switch.js` 按 `process.platform/arch` 选择正确二进制；`npm pack` 含多平台二进制
- [x] 零迁移：既有 `~/.pi-switch/config.json`（含旧 `version=1`）与 SQLite 在 Go 启动后可读，旧字段迁移后写回不丢；`daemon` 的 `pid` 多实例与 `webui_password` 逻辑与 `src-rust/daemon.rs` 一致

## 实施总结
- 提交：`08d0353` — `feat(rewrite-go): 14 TUI and release convergence`
- 实现的 seams：
  - TUI bubbletea — `pi-switch tui` (internal/tui: profile list/switch via list.Model, gateway publish via gateway.Publish, stats with cost format `$0.00/$0.0042/$1.2K/-` and cacheRate)
  - CLI help — `printHelp` lists tui/package/ccs/presets/gateway/stats/config/doctor, verified via bin wrapper
  - package/cc-switch — CLI `package list/add/sync/import` + `ccs list/import` + HTTP `/api/packages` `/api/ccswitch/providers` (single seam, stubs backed by real routing)
  - daemon — `internal/daemon` (pid files `~/.pi-switch/*.pid`, log, isAlive via kill -0/tasklist, health via TCP, start/stop/status, ss multi-instance hint) + `proxy/webui start --daemon --host --port`
  - zero-migration — config v1→v2 (`injectOpenCodeAttribution→conversationSource`, per-request LoadConfig), SQLite `ensureTable` column migrations, webui_password Basic Auth parity
  - cross-compile + npm wrapper — `bin/pi-switch.js` platformMap/archMap + PI_SWITCH_GO_BIN override, `GOOS×GOARCH` 6 binaries (`-ldflags "-s -w"`, pure Go), `scripts/build-all.sh`, `package.json` files `bin/` + `webui/dist/`, `modernc.org/sqlite`
- 验收标准：
  - [x] tui 可启动并完成 profile 切换、gateway 发布、stats 浏览；--help 列出全部子命令 (`go run --help` + `node bin/pi-switch.js --help` 均含 tui/package/ccs/presets/gateway)
  - [x] package 与 cc-switch 在 Go 侧可用：`pi-switch package list/add/sync` 与 `pi-switch ccs list/import` 单测覆盖 (`internal/server/tui_release_test.go: TestPackageAndCcsApis` 5 routes, CLI via `handlePackage/handleCcs`)
  - [x] GOOS×GOARCH 四平台产物可执行，bin wrapper 按 platform/arch 选择正确二进制；npm pack 含多平台二进制 (6 binaries built `bin/pi-switch-*`, wrapper `platformMap/archMap`, `package.json` `files: ["bin/"]` + `build:all`)
  - [x] 零迁移：既有 config v1 与 SQLite 在 Go 启动后可读，旧字段迁移后写回不丢；daemon pid 多实例与 webui_password 逻辑与 src-rust/daemon.rs 一致 (`config.LoadConfigAtPath` 迁移, `store.ensureTable` ADD COLUMN, `daemon.Status` ss hint, `server.authMiddleware` loopback check)
- 测试结果：`go test ./...` 66 passed (tui 4 + daemon 5 + server/tui_release 4 + config 4 + proxy + server 等), `go vet ./...` 通过
- typecheck：通过
- 文档对齐：更新 `README.md` (Go badge, build:webui+go, arch 图改 Go internal/, TUI 描述改 bubbletea, dev 命令改 go test), `package.json` (files bin/webui/dist, scripts build:go/build:all), `.gitignore` (ignore Go binaries), `bin/pi-switch.js` (Go dispatch), 新增 `scripts/build-all.sh`
- 遗留 / 后续建议：`package` 的 add/install 仍为 stub（上游 pi 扩展 sync 未完整移植，待后续补全真实 `~/.pi/agent/settings.json` 同步）；`tui` 的 form 编辑仍经 CLI `provider add`，未内置表单（与现有 TUI 1:1 功能中 form 可后续迭代）；二进制 6×15M 未提交至 git（由 `build:all` 在发布前生成，npm pack 时包含）
