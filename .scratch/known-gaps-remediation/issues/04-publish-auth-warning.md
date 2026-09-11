# 04: F21(A)：暴露代理时明确告知 published provider 无法鉴权 + 文档化

Status: resolved (2026-09-11)

**What to build:** 当监听绑定**超出 loopback**（即代理面要求 HTTP Basic）时，
`POST /api/gateway/publish` 的成功响应新增 `warnings`，说明刚发布的 provider 无法通过鉴权；
两个 README 记录该限制。

**为什么**：`gateway.go:236` 给每个发布的 provider 写死 `"apiKey": "pi-switch-proxy"`，
baseUrl 指向代理（`:184`）；而代理守卫只接受 `Authorization: Basic …`（`server.go:486-490`）。
于是"网关自己配置出去的那个客户端"在非 loopback 绑定下必然 401。默认部署看不见它，因为通配绑定
被改写成 `127.0.0.1`（`gateway.go:178-181`）且 loopback 不设防。

**方案 A 的三条约束（用户已选，必须守住）**
1. 不把共享密码写进 `models.json`（P0 批明令）。
2. 不引入新的可发布凭据（那是 B 案，另立票）。
3. 明确告知 + 文档化，不做静默的"看起来好了"。

**为什么不需要密码就能判定**：`ValidateBindAuth`（`server.go:377`）拒绝"非 loopback 且无密码"
的启动，因此**非 loopback ⇒ 一定有密码 ⇒ 一定装了守卫**。结合 P0 批的
`requestAuthOptions(c)` + `effectiveBindHost`（空 host 按通配读，不按 loopback），
判定不需要触碰密钥（密码本就被刻意从 context 里抹掉）。

**已知不精确处（诚实记录）**：判定用的是**本进程**的绑定地址；若操作者用不同 `--host` 分别启动
webui 与 proxy，警告可能与代理面的真实姿态不符——但它只会误报、不会漏报，属安全方向。

**Seam**：真实 HTTP（publish 响应）；CLI `gateway publish` 把 warnings 打到 stderr。

**验收**
- [ ] loopback 绑定 → 无 warnings（默认部署不被噪音打扰）
- [ ] 非 loopback 绑定 → 有 warnings，且说明原因与出路（需 Basic 客户端）
- [ ] 响应中不含任何密码/凭据
- [ ] 两个 README 记录该限制
- [ ] 既有 publish 行为（成功写盘、冲突 400）不变

**勘误（2026-09-11，`review-remediation/08`）**：本票在 spec 的验收框里写「`webui/src/api.ts`
的 `proxyStart` 无调用点（已核）」——**假的**。`webui/src/components/ProxyPanel.tsx:66` 调用它；
我当时 grep 的是错误的标识符（`startProxy`，实际叫 `proxyStart`）。
可达性：清空 host 输入框后点 Start → body 变成 `{"host":""}` → 命中该分支（今日返回 501）。
前端影响：`webui/src/api.ts:75` 对**对象型** `error` 退回用 `res.statusText`，故用户看到
"Not Implemented"，可操作的 message 在 UI 里不可见（其余 7 个 501 端点同理）。
正解是让前端读 `error.message`，改它需跑 tsc/vitest，列为后续项。

**测试**：`internal/server/gateway_publish_warning_test.go`
