# 01: `POST /api/proxy/start` 非 daemon 分支不再谎报成功

Status: resolved (2026-09-11)

**What to build:** 空 body（无 `daemon`、无 `?daemon`、host/port 均为空）的 `POST /api/proxy/start`
不再返回 `{"running": true, "message": "proxy started (stub)"}`，而是按 P0 批已确立的
`notImplemented` 形状返回 **501**，并在 message 里说清怎么才能真正启动。

**为什么**：`settings_handlers.go:127` 是纯字面量——该分支没有 `daemon.Start`、没有任何监听、
没有启动任何进程。它恰好命中"什么参数都没给"的调用，却回答"代理已启动"。P0 批的全局判据
「不存在返回 200 但无副作用的端点」在**全仓范围**正因这三处而不成立。

**取舍（记录）**：选 501 而非 400——该能力（进程内启动代理）本来就没实现，`notImplemented`
形状与 `config backups`/`ccinit` 等既有 501 一致；400 会暗示"参数错了"。

**Seam**：真实 HTTP（`NewMgmtRouter` + `callMgmt`）。

**验收**
- [ ] 空 body → 501，body 是 `{"error":{"type":"not_implemented",...}}`，且**不含** `running: true`
- [ ] message 指明 `daemon`/host/port 是启动的前提
- [ ] `{"daemon":true,...}` 的既有路径不受影响（`tui_daemon_test.go` 保持绿）
- [ ] `webui/src/api.ts` 的 `proxyStart` 无调用点（已核），故此改动不破坏前端

**测试**：`internal/server/proxy_start_honesty_test.go`
