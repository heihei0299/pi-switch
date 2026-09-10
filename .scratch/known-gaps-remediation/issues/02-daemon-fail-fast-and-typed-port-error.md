# 02: daemon 启动：子进程已死即失败 + 端口占用改为类型化错误

Status: resolved (2026-09-11)

**What to build:**
1. `daemon.Start` 的健康等待在**子进程已退出**时立即结束，不再把剩余尝试等完。
2. 导出 `daemon.ErrPortInUse`，端口被占用时用它包装错误；handler 用 `errors.Is` 判定，
   不再 `strings.Contains(err.Error(), "port already in use")`。

**为什么**：
- 等待时长按机制算 = 15 次 ×（`/healthz` + `/health` 两次探测 × 500ms 超时 + 200ms 间隔）。
  端口被占用时 TCP 能连上、健康探测却非 2xx → **吃满约 18 秒**；连接被拒时约 3 秒。
  而这两种情况下子进程**早就退出了**，继续等不可能改变结论（`daemon.go:450`）。
- `settings_handlers.go:117` 按错误文本判断"端口被占用"，与上一批次刚从 profile 域移除的做法同类。
- 既有测试 `tui_daemon_test.go:106` **没有真的触发 EADDRINUSE**——它预先写入含该字样的假日志
  （`:116` 注释自认），所以它锁定的是"按文本嗅探"这条路径本身。

**实现要点**：只改 `Start` 内的轮询循环（`managedHealth(info, 1)` + `isAlive(info.Pid)` 早退），
不改 `checkHealth`/`checkHealthEndpoint`/`managedHealth` 的签名——`managedHealth` 另有 2 个
调用点（`:399`、`:534`，状态查询）不该受影响。`isAlive(0)` 会 `kill -0 0`（信号发给整个进程组），
故 pid 为 0 时不做存活判定。

**Seam**：`internal/daemon` 的 `Start` 返回错误 + 真实 HTTP。

**验收**
- [ ] 子进程立即死亡时，`daemon.Start` 在 ~1 秒内返回错误（不再等满 15 次）
- [ ] 端口占用的错误满足 `errors.Is(err, daemon.ErrPortInUse)`
- [ ] 消息仍含 `port already in use` 与 `ss -tlnp`（既有测试的断言不变）
- [ ] `isAlive(0)` 不被调用；状态查询两处行为不变
- [ ] handler 不再出现按错误文本分类

**测试**：`internal/daemon/start_failfast_test.go`、`internal/server/proxy_start_honesty_test.go`
