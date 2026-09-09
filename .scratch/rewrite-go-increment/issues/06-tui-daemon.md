## Question

`TUI` 与 `daemon` 的后端契约应如何设计？`charmbracelet/bubbletea` 的 `list.Model` 与 `daemon` 的 `pid` 文件、`kill -0`/`TCP health` 的跨平台抽象，以及 `proxy/webui start --daemon --host --port` 的多实例提示？

需决策：
1. TUI：`bubbletea` 的 `list.Model`（`profile` 列表/enter 切换）与 `gateway publish`（`timer` 轮询 `gateway` 状态）与 `stats`（`FilterKey` + `cost` 格式化 `$0.00/$0.0042/$1.2K/-`）的 `tea.Msg` 流，`FilterKey` 的 `input` 与 `list` 的 `filter` 竞争
2. Daemon：`internal/daemon` 的 `~/.pi-switch/*.pid`（`proxy.pid:port:host:pid`）与 `*.log` 的 `kill -0`（`unix`）/`tasklist`（`windows`）的 `isAlive`，`TCP health` 的 `net.DialTimeout` 与 `ss -tlnp` 多实例 `500` 提示
3. 启动：`proxy/webui start --daemon --host --port` 的 `daemon.Start` 的 `exec.Command` 的 `Stdout/Stderr` 到 `*.log`，`health` 的 `retry` 与 `port` 占用 `EADDRINUSE` 的 `500` 区分
4. 跨平台：`bubbletea` 的 `FilterKey` 在 `windows` 的 `conhost` 的 `Alt` 键与 `daemon` 的 `tasklist` 的 `Image Name` 的 `go` 二进制名（`pi-switch-go` vs `pi-switch`）的一致性

## Type

grilling

## Status

resolved

## Blocked by

01

## Answer

**决策总览**：以现有 `internal/tui/model.go + run.go` 的 `list.Model + 3-tab` 与 `internal/daemon/daemon.go` 的 `~/.pi-switch/*.pid|*.log + kill -0/tasklist + net.DialTimeout` 为既成实现，补齐 `tea.Msg` 时序与多实例 `EADDRINUSE` 区分，不引入 `fsnotify` / `systemd` 等外部依赖，保持 `go vet + go test (tui + daemon) + webui 5` 回归可测。

1. **TUI：`bubbletea + list.Model` 的 `tea.Msg` 流，`FilterValue` 与 `FilterKey` 的隔离，gateway/stats 非流式轮询**
   - 模型：`type Model struct { cfg config.PiSwitchConfig; list list.Model; tab int(0:profiles|1:gateway|2:stats); statusMsg, statsBrief, gatewayPreview string; width,height int }`（`tui/model.go:Model`）。`profileItem` 实现 `list.Item`（`Title() "● name (current)"| "  name"`、`Description() "api=.. models=.. exposed=.." `、`FilterValue() string=name`）供 `list.Model` 的 `/` 过滤匹配 `name`。
   - 初始化：`New(cfg)` 构造 `items`（`current` 高亮 `●`），`list.New(items, delegate, 60,14)`，`Title="Profiles — enter to switch, g=gateway, s=stats, q=quit"`，`SetShowHelp(true)`，随即 `refreshGateway()`（`gateway.BuildProposedGatewayEntry(cfg)` 合成 `Gateway <prefix> @ <host>:<port> models=N`）与 `refreshStats()`（见下）预填 `View()` 的 `1:profiles|2:gateway|3:stats` 三 tab 首帧，避免 `Init() tea.Cmd==nil` 时的首屏空窗。
   - `tea.Msg` 流（`Update(tea.Msg)`）：
     ```
     WindowSizeMsg → SetSize(width-4,height-8)
     KeyMsg "q/ctrl+c/esc" → Quit
     "1/2/3" → tab=0/1/2
     "g" (tab==1) → gateway.Publish(cfg, BuildProposedGatewayEntry(cfg)) 直写 `models.json:providers[pi-switch]`，成功 `statusMsg="gateway published"` + `refreshGateway()`，失败 `statusMsg="gateway publish failed:.."`（错误隔离：publish 失败不影响 `list`）
     "s"/"r" → refreshStats() + statusMsg="stats refreshed"
     "enter" (tab==0) → 取 `m.list.SelectedItem().(profileItem)`，`cfg.Current=&name`  → `saveConfig(cfg, configPath())`（原子 `tmp+rename`），成功重建 `list.SetItems` 并高亮 `current`，`statusMsg="switched to <name>"`；失败保留旧 `current`
     其他 → 若 tab==0 透传 `m.list.Update(msg)`（列表光标 ↑/↓、`/` 过滤的输入均在此分支），tab==1/2 直接 `return m,nil`
     ```
     `Init()` 保持 `nil`（不启动 `tea.Tick`）；`gatewayPreview/statsBrief` 均为同步 `refresh*()` 填充，**不设 `timer` 3s 轮询** —— 原因：`sessionScan` 3s 轮询与 `gateway.notify` 文件通知均在代理侧，TUI 仅以 `g/r/s` 手动触发为准，避免后台 `Tick` 与 `list` 的异步 `tea.Cmd` 竞争导致闪烁；若后续需实时，优先在 `internal/server` 暴露 `GET /api/gateway/preview` 的 `pending_count` 并由 TUI 在 `g` 后 `refreshGateway()` 即可，不引入常驻 `Tick`。
   - `FilterKey` 竞争：`list.Model` 的默认键 `"/"` 进入过滤输入态，此时 `input` 捕获键盘；TUI 的 `enter` 切换仅在非过滤态生效（`list.FilterState()==Filtering` 时 `enter` 交 `list.Update` 处理，避免与 `profile` 切换冲突）。`windows conhost` 的 `Alt` 组合由 `bubbletea` 自身屏蔽，不自定义 `FilterKey`；`helpStyle` 提示 `↑/↓ navigate • enter switch • g publish • s refresh • 1/2/3 tabs • q quit`，过滤提示沿用 `list` 默认 `"Filtering..."`。
   - `View()`：`headerStyle("pi-switch TUI — 1:profiles  2:gateway  3:stats  q:quit") + "─"*60`，`tab==0: m.list.View()`，`tab==1: titleStyle("Gateway")+gatewayPreview+" Press g to publish gateway → writes models.json:providers[pi-switch]"`，`tab==2: titleStyle("Stats")+statsBrief+" Press r/s to refresh. Total cost shows '-' when unknown, else $0.00 / $0.0042 / $12.34 / $1.2K"`，`statusMsg` 以 `selectedStyle("▶ "+msg)` 居底，`helpStyle` 常驻。此布局与 `tui_test.go: TestTUI_RendersProfiles/GatewayAndStats/EmptyConfig` 的外部可观测断言一致。
   - `statsBrief` 成本格式化（与 `CONTEXT.md: 消费` 对齐）：`SELECT SUM(cost) FROM requests WHERE success=1` 聚合（仅成功且可归因行），`0 → $0.00, 0.0042 → $0.0042 (≤4 位), 12.34 → $12.34, ≥1000 → $1.2K, nil/0 且 cnt==0 → $0.00, cost==0 且 cnt>0 → -`（与 `webui: formatCost` 映射等价），`cacheRate` 为 `cached/input` 的 `"-" | "0.0%" | "x.x%"`。

2. **Daemon：`~/.pi-switch/*.pid + *.log` 的 `isAlive + TCP health` 双探活，`ss -tlnp` 多实例 500 提示**
   - 路径：`configDir()` 优先 `PI_SWITCH_CONFIG_DIR` 环境，否则 `~/.pi-switch`（`os.UserHomeDir()` 失败回退 `/tmp/pi-switch`），`pidPath=join(configDir, proxy.pid|webui.pid)`，`logPath=join(configDir, proxy.log|webui.log)`。`Service` 枚举 `Proxy{pid:proxy.pid, log:proxy.log, subcommand:proxy}` 与 `WebUI{webui.pid, webui.log, webui}`（`daemon.go:ServiceByName`）。
   - 文件格式：`pid` 文件为 JSON `DaemonInfo{pid:uint32, host:string, port:uint16, startedAt:uint64(epoch millis)}`（`json.Marshal` 后 `0644`），`log` 文件为追加式文本（`O_CREATE|O_APPEND|O_WRONLY`），`StartedAt` 供 `Status` 展示存活时长。无 `flock` 依赖，原子写仅对 `pid` 的 `WriteFile`，`log` 追加不锁。
   - `isAlive(pid)`：`unix: exec.Command("kill","-0",pid).Run()==nil`，`windows: exec.Command("tasklist","/FI","PID eq <pid>","/NH","/FO","CSV").Output()` 解析第二列 `","<pid>","` 是否存在；两者均无权限提权，回退 `false` 触发 stale 清理。
   - `checkHealth(host,port,attempts=2)`：`net.DialTimeout("tcp", net.JoinHostPort(host,port), 500ms)`，成功 `Close` 即活，失败 `sleep 200ms` 重试 `attempts` 次；`attempts=2` 兼顾 `proxy` 的 `gin` 监听延迟与测试收敛（`daemon_test.go: TestCheckHealth_False` 覆盖）。
   - `Status(Service)` 双探活顺序：`readPidFile==nil → running=false, msg="no PID file"`；`isAlive==false → removePidFile + running=false, msg="PID <n> is not alive (cleaned up)"`；`isAlive==true && !checkHealth → removePidFile + running=false, msg="process exists but port <host:port> not responding. Cleaned up stale PID."`；`isAlive && checkHealth → running=true, pid/host/port/startedAt, msg="Proxy|WebUI daemon is running (PID <n>) on http://<host:port>"`，`linux` 时附加 `ss -tlnp` 多实例提示：`strings.Count(ssOutput, ":<port>")>1` 则 `msg+=" (note: <count> listeners on :<port> — use ss -tlnp to locate)"`（`daemon.go:Status` 已实现）。此分级使 `stale pid`（进程已退但文件遗留）与 `port not responding`（进程挂起）均自动清理，不误报 `running`。
   - 平台差异：`conhost` 的 `tasklist` 输出为 `CSV` 含 `Image Name` 列，`*Image Name*` 固定为 `pi-switch`（`go build -o bin/pi-switch` 产物，`windows` 追加 `.exe`），不以 `pi-switch-go` 命名，保证 `tasklist` 的第二列 `PID` 判活与 `Status` 的 `msg` 中 `PID` 一致。

3. **启动：`proxy/webui start --daemon --host --port` 的 `exec.Command → *.log`，`health retry` 与 `EADDRINUSE` 的 500 区分**
   - `Start(Service, host, port)` 幂等：先 `readPidFile`，若 `isAlive && checkHealth(2)` 则直接返回 `running=true, "already running on http://<host:port>"`（不重复拉起）；否则 `removePidFile` 清理后继续。
   - 参数归一：`host=="" → "127.0.0.1"`，`port==0 → proxy:43112, webui:43110`（与 `config.DefaultConfig` 的 `proxy.host/port` 与 `web.host/port` 默认一致），`os.MkdirAll(configDir,0755)`。
   - 拉起：`exec.Command(os.Executable(), subcommand, "internal-start", "--host", host, "--port", strconv.Itoa(port))`（实际入口为 `cmd/pi-switch` 的 `proxy/webui` 子命令的内部分支，避免 `bin/pi-switch.js` 的 Node 包装），`Stdout/Stderr` 均 `OpenFile(logPath, append)`，`proc.Start()` 后即写 `DaemonInfo{Pid:uint32(proc.Pid), Host:host, Port:port, StartedAt:nowMillis}` 至 `pidPath`。`detached` 由 `proc.Process.Release()` 实现，父退出不杀子（`unix` 不依赖 `setsid`，`windows` 由 `taskkill /F /PID` 兜底）。
   - 健康重试：`checkHealth(host,port, attempts=15, 200ms间隔 ≈3s)`；`attempts` 内成功 → `running=true, "started on http://<host:port> (PID <n>)"`；超时 → `Stop(Service)` 清理并返回 `running=false, error="health check failed after <n> attempts: ..."`（避免 `proxy.pid` 指向未就绪进程）。
   - 端口占用区分：若 `health` 探测期间捕获 `listen: address already in use`（`EADDRINUSE`）的 `log` 尾部（`Tail(logPath, 2KB).Contains("address already in use"||"bind:")`），则返回 `running=false, status=500语义, msg="port <port> already in use — use ss -tlnp (linux) / netstat -ano (windows) to locate, or pi-switch proxy stop && proxy start --port <alt>"`，与健康超时区分；前端 `api.ts` 的 `POST /api/proxy/start → 500` 映射为 `toast err` 的 `EADDRINUSE` 提示（与 2 的 `ss -tlnp` 计数联动）。
   - `Stop(Service)`：`readPidFile==nil → running=false, "not running"`；`isAlive==false → removePidFile + running=false, "cleaned stale"`；`isAlive==true → proc.Kill()`（`unix: kill <pid>`，`windows: taskkill /F /PID <pid>`），`poll 20*100ms` 至 `!isAlive` 则 `removePidFile` 返回 `running=false, "stopped"`，超时则 `kill -9` / `taskkill /F` 强制杀后清文件，`msg` 含 `force killed`。

4. **跨平台：`bubbletea` 的输入与 `daemon` 的二进制名一致性**
   - `bubbletea` 在 `windows conhost` 的 `Alt` 键由库内部 `readConsoleInput` 过滤，TUI 不自定义 `key.Map` 的 `FilterKey`，仅用 `q/ctrl+c/esc` 退出、`1/2/3` 切 tab、`g/s/r/enter` 业务键，避免 `Alt` 误触发过滤。
   - 二进制名统一为 `pi-switch`（`go build -o bin/pi-switch`，`windows` 自动 `bin/pi-switch.exe`），不使用 `pi-switch-go` 别名，使 `ps aux | grep pi-switch`、`tasklist /FI "IMAGENAME eq pi-switch.exe"` 与 `daemon.go: isAlive` 的 `tasklist` 解析一致，`webui.pid` 与 `proxy.pid` 的 `message` 中的 `PID` 可直接 `kill`/`taskkill`。`pnpm store` 的 `links/@heihei0299/pi-switch` 快照中的 `.node` 与本 Go 二进制同名区分（`.node` 为 NAPI，`pi-switch` 为 Go），不混淆。
   - `daemon` 的 `pid` 文件仅记录最后一次 `start --daemon` 的实例（单 `*.pid` 文件语义），多 `webui` 进程并存时 `pi-switch webui stop` 仅停 `pidFile` 指向的进程，旧实例需 `ss -tlnp` 定位后 `kill`（与全局 `AGENTS.md` 的 `daemon pid 文件只记录最后一个启动实例` 约束一致）。

   **可测试性**（最高缝 `go test ./...` + `NODE_ENV=test vitest`）：`internal/tui` 的 `RenderView/Header/list/gatewayPreview/statsBrief/cost` 外部可观测（`tui_test.go: TestTUI_RendersProfiles/GatewayAndStats/EmptyConfig/SwitchViaFile`），`internal/daemon` 的 `ServiceByName/PidFileLifecycle/StaleCleanup/Status_NoPidFile/CheckHealth_False` 与 `go vet` 的 `TestMode` 隔离；`internal/server` 的 `POST /api/proxy/start --daemon` 的 `already running` 幂等、`EADDRINUSE → 500` 与 `proxy/webui start --host/--port` 参数透传在 `gin.TestMode + httptest` 覆盖，保留 `66 passed + webui 5` 基线。
