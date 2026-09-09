# 11: TUI 与 Daemon 常驻与多实例

**What to build:** 让 `pi-switch tui` 与 `proxy/webui start --daemon --host --port` 在终端可演示：`bubbletea` 的 `list.Model + 3-tab` 可切换/发布/刷新，`~/.pi-switch/*.pid|*.log` 的双探活与 `EADDRINUSE 500` 多实例提示在 `go test` 与 `httptest` 可断言。

**Blocked by:** 10 WebUI 同步与实时统计

**Status:** resolved

- [x] `internal/tui/model.go: type Model{cfg,list,tab,statusMsg,statsBrief,gatewayPreview}`，`profileItem{Title:"● name (current)", Description:"api=..", FilterValue=name}`，`New` 构 `list.New(60,14) Title="Profiles — enter..."` 并 `refreshGateway(BuildProposedGatewayEntry→"Gateway <prefix> @ <host:port> models=N") + refreshStats(SUM(cost)→$0.00/$0.0042/$1.2K/-)`
- [x] `Update tea.Msg: WindowSize→SetSize / q,ctrl+c,esc→Quit / 1,2,3→tab / g(tab1)→gateway.Publish→models.json / s,r→refreshStats / enter(tab0)→SelectedItem→cfg.Current=&name→saveConfig tmp+rename→SetItems高亮`，其余 `tab0→list.Update`，`Init()=nil` 不开 `Tick`，`FilterState==Filtering` 时 `enter` 交 `list`，`View` 分 `0:list /1:Gateway+g /2:Stats+r` + `statusMsg(▶)`，`RenderView` 与 `SwitchViaFile` 在 `tui_test.go` 覆盖
- [x] `statsBrief Cost: "-" | $0.00 | $0.0042 | $12.34 | $1.2K`（`cnt==0→$0.00, cnt>0&&cost==0→-, cost<0.01→$%.4f`），`cacheRate "-" | "0.0%" | "x.x%"`
- [x] `internal/daemon: configDir(PI_SWITCH_CONFIG_DIR||~/.pi-switch) → proxy.pid/webui.pid(JSON DaemonInfo{pid,host,port,startedAt}) + proxy.log/webui.log(append)`，`isAlive: kill -0 / tasklist CSV第2列`，`checkHealth: DialTimeout 500ms×2`
- [x] `Status: nil→not running / !isAlive→rm stale / isAlive&&!health→rm stale / isAlive&&health→running + linux ss -tlnp :port>1 追加提示"`，`Start: isAlive&&health→already running` 否则 `127.0.0.1:43112/43110` → `exec.Command → *.log → Release → health 15×200ms`，`EADDRINUSE`（`log tail "address already in use"`）→ `500 port already in use — use ss -tlnp` 与超时区分，`Stop: Kill→20×100ms poll→rm→超时 kill -9/taskkill /F`，`ServiceByName/PidFileLifecycle/StaleCleanup` 在 `daemon_test.go` 覆盖
- [x] 二进制名统一 `pi-switch[.exe]`（不叫 `pi-switch-go`），`FilterKey Alt` 由 `bubbletea conhost` 屏蔽，`*.pid` 单文件仅记最后实例（多 webui 并存需 `ss -tlnp` 定位 `kill`），`POST /api/proxy/start --daemon` 的 `already running` 与 `EADDRINUSE 500` 在 `gin.TestMode httptest` 覆盖

## 实施总结
- 提交：`f88d292` — `feat(rewrite-go-increment): tui daemon and multi-instance (#11)`
- 实现的 seams：S1 tui Model+list, S2 Update/View, S3 Cost, S4 daemon pid/log, S5 Status/Start/Stop, S6 跨平台与 API
- 验收标准：6/6 全选
- 测试结果：go test ./internal/tui 7/7；./internal/daemon 7/7；./internal/server 5/5；go test ./... 全绿
- typecheck：go vet 通过
- 文档对齐：CONTEXT 术语未变；issue 已 resolved
- 遗留 / 后续建议：无；多 webui 并存仍需 ss -tlnp 手动 kill
