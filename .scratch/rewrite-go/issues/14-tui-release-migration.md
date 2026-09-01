# 14: TUI 与发布收敛（bubbletea + package + 构建分发 + 零迁移）

**What to build:** 收敛剩余能力并打通发布：`charmbracelet/bubbletea` + `bubbles` + `lipgloss` 重写 `ratatui` TUI（`pi-switch tui` 子命令，`profile` 列表/切换、`gateway` 状态、`stats` 简览含 `totalCost/-`），`package` 管理（`catalog/package/package_ops` 的 `add/install/uninstall/enable/disable/sync/import_from_pi`）、`cc-switch` 导入（`listCcsSwitchProviders/importCcsSwitchProviders`）、`preset`/`sync`/`scan_pi`/`credits` 等能力在 Go 中保持可用；多平台交叉编译 `GOOS=linux/darwin/windows×GOARCH=amd64/arm64`（`modernc/sqlite` 纯 Go 无 CGO，`go build -ldflags "-s -w"`），`bin/pi-switch.js` 按平台分发执行对应 Go 二进制，继续发布 `@heihei0299/pi-switch` 到 npm（`pnpm` 全局安装为 store 快照，需复制 `.node→Go` 二进制到快照或重装，`config.json` 热重载无需重启），`~/.pi-switch`（`config.json`、SQLite `requests.db`、`webui_password`、备份）零改动兼容，`daemon` 的 `pid` 文件（`~/.pi-switch/*.pid`）与 `proxy/webui start --daemon --host --port` 语义保持（`stop/status` 含多实例 `ss -tlnp` 定位提示）。

**Blocked by:** 13 WebUI 管理面

**Status:** ready-for-agent

- [ ] `pi-switch tui` 可启动并完成 `profile` 切换、`gateway` 发布、`stats` 浏览；`--help` 列出全部子命令
- [ ] `package` 与 `cc-switch` 导入在 Go 侧可用：`pi-switch package list/add/sync` 与 `pi-switch ccs list/import` 单测覆盖
- [ ] `GOOS×GOARCH` 四平台 `go build` 产物可执行，`bin/pi-switch.js` 按 `process.platform/arch` 选择正确二进制；`npm pack` 含多平台二进制
- [ ] 零迁移：既有 `~/.pi-switch/config.json`（含旧 `version=1`）与 SQLite 在 Go 启动后可读，旧字段迁移后写回不丢；`daemon` 的 `pid` 多实例与 `webui_password` 逻辑与 `src-rust/daemon.rs` 一致
