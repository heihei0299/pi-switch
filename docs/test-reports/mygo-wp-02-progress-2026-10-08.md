# WP-02 本地实现进展 — 2026-10-08

> 状态：实现中；Gate A 仍 BLOCK。用户明确授权先做本地实现，不代表豁免验收或可进入 WP-03/WP-04。

## 范围与修改

- `desktop/main.go`, `desktop/shell.go`: MyGo 单实例恢复、关闭隐藏/显式退出、托盘菜单与无托盘应用菜单回退；支持仅在 tray 成功时 `--start-minimized`。
- `desktop/shell.go`: 通过现有 `internal/daemon` 查询、启动和停止 Proxy；操作运行在后台，桌面退出不调用 Proxy stop。
- `internal/daemon/daemon.go`: 为 `Service` 增加可选 executable，保持既有调用默认使用当前可执行文件；桌面可指定 CLI 子进程，避免误启动自身 GUI。修正 Linux `ss` 参数 `"H"`→`"-H"`，避免误入耗时的 `/proc` listener fallback；保留 fallback。
- `desktop/shell_test.go`, `internal/daemon/daemon_test.go`: 覆盖 CLI 路径、菜单入口、tray 缺失时启动可见、输入回显及 executable 选择。
- 应用仍使用独立 spike app ID；没有读取真实配置、请求数据库、模型注册表或 Pi session。

## 验证

| 命令 | 结果 |
| --- | --- |
| `GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 go test -mod=readonly ./...`（`desktop/`） | PASS |
| `GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 go vet -mod=readonly ./...`（`desktop/`） | PASS |
| `./node_modules/.bin/mygo vet .`（`desktop/`） | PASS |
| `GOTOOLCHAIN=local GOPROXY=off go test -mod=readonly ./internal/daemon`（临时 `PI_SWITCH_*` 路径） | PASS（4.0s）；包含之前超时的启动失败/端口占用用例 |
| `GOTOOLCHAIN=local GOPROXY=off go vet -mod=readonly ./internal/daemon` | PASS |
| 定向 daemon 身份/端口/锁测试（临时 `PI_SWITCH_*` 路径） | PASS |
| `git diff --check` | PASS |

## 未验收 / 风险

- Linux tray 实机、关窗后 HTTP 健康检查、显式停止/重复启动、崩溃后代理存活尚未做集成验收。
- fcitx5/IME、高 DPI、Wayland 降级、Windows/macOS GUI、启动性能仍 BLOCK；未启动 GUI，也未做发布构建。
- 启动 Proxy 需要同版本 CLI 可执行文件，暂从 app resources、`PATH` 或 `PI_SWITCH_CLI_PATH` 查找；WP-07 前尚未打包 CLI。
- 未完成 code review，未提交；WP-03/WP-04 未开始。
