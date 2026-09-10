# 07 — Stats 当前窗口会漏掉刚完成的请求

**现象：** 代理请求已经成功写入 SQLite 和 `requests.log`，但 WebUI Stats 在请求完成后立即查询当前时间窗口时，统计数量为 0；将查询窗口结束时间向后放宽约 1 秒后，同一请求才出现。

**复现步骤：**

1. 在 `pi-switch-web` 容器内用新二进制启动隔离 proxy 与 WebUI。
2. 通过真实 mock upstream 完成一条 `model-a` chat completion。
3. 立即请求：

```text
GET /api/stats?range=last24h&from=<now-86400000>&to=<now>&page=0&limit=50
```

4. 再用相同 `from`，但将 `to` 改为 `<now+1000>` 查询。

**预期：** 请求完成后，只要请求的真实发生时间落在窗口内，当前 Stats 窗口立即应包含该请求。

**实际：** 本轮真实容器证据：

```text
request ts = 2026-09-09T20:26:09Z
to         = 2026-09-09T20:26:09.000Z
/api/stats => totalRequests: 0
```

将 `to` 向后移动 1000ms 后：

```text
/api/stats => totalRequests: 1
```

SQLite 中该请求行已经存在，且无窗口 Stats 查询可以读到它；因此不是写入失败或数据库延迟。

**根因：** 请求日志在 `internal/server/server.go` 中使用秒精度的 `time.Now().Format(time.RFC3339)` 写入 `ts`，而 `webui/src/lib/statsWindow.ts` 与 Stats API 使用毫秒精度的右开窗口 `[from,to)`。请求发生在当前秒内时，持久化时间会被截断到当前秒，恰好等于 `to`，被 SQL 的 `< to` 条件排除。

**证据：** 本轮 `incus` 容器内真实 proxy → SQLite → WebUI Stats API 测试；同一条请求仅改变 `to` 的 1000ms 余量，结果从 0 变为 1。

**Blocked by:** None — can start immediately

**Status:** ready-for-agent

- [ ] 请求时间持久化精度与 Stats 窗口精度一致，或窗口边界不会漏掉当前秒请求
- [ ] 真实请求完成后立即刷新 Stats 能看到该请求
- [ ] 右开窗口语义仍对严格等于 `to` 的历史行保持一致
- [ ] 回归测试覆盖同秒请求、毫秒窗口和 1 秒边界

## 建议修复方向

优先保留请求真实时间的毫秒/纳秒精度（例如 `RFC3339Nano`），并在 SQLite 查询中保持明确的 epoch-millisecond 右开语义；补充一个 `ts` 与 `to` 同秒但真实毫秒早于 `to` 的回归测试，再用真实 WebUI 立即刷新复测。

## Comments

- 无窗口查询和延迟 1 秒后的查询均可读到数据，问题集中在“当前窗口立即查询”的时间精度边界。
- 本问题与模型暴露保存失败独立；两者均会影响 WebUI 用户对“保存/请求后是否生效”的判断。
- 本机 `agent-browser` 复测：真实 proxy 完成 non-stream/stream 请求后，按实际持久化 `ts` 精确查询得到 `totalRequests=0`，将 `to` 向后放宽 1000ms 后得到 `totalRequests=2`；Stats 页面普通刷新可见 2 行。本次没有使用 `/api/*` route mock。
