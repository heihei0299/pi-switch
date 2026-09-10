# 02: Proxy 认证与请求体上限

**What to build:** 让 Proxy 面与非 loopback 绑定时的信任边界和 01 保持一致，并给请求体加上限。端到端行为：非 loopback 启动 Proxy 时无凭据请求被拒；超大请求体返回 413；loopback 本地开发行为不变。

**实现要点**（spec D1 的延伸）：

- `NewProxyRouter` 在非 loopback 时挂**同一套** Basic 认证（复用 01 的认证模式与密码来源，不新建第二套机制）
- proxy 入口加 `http.MaxBytesReader`：上限可配，缺省 32MB
- `internal/server/proxy_handlers.go:248` 是唯一读体点（`io.ReadAll(c.Request.Body)`），改造面应局限于此
- 若命中上限，返回 413 且不落库、不计费

**Blocked by:** 01: 认证判定改用实际绑定地址，非 loopback 默认拒绝启动

**Status:** resolved (2026-09-11)

- [x] 非 loopback + 无密码时 Proxy 不可用（启动失败或请求被拒）；有密码时无凭据请求返回 401
- [x] 请求体超过上限返回 413，且不产生请求落库/计费记录
- [x] 上限可配置，缺省 32MB；loopback 场景行为与改动前一致
- [x] 认证机制与 01 共用同一份实现（不出现第二套认证代码或第二处密码读取）
- [x] 测试：非 loopback + 无密码 → proxy 拒绝；超大 body → 413；loopback 正常路径不回归
- [x] `go build ./...` 通过；`go test ./... -count=1` 全绿；`gofmt -l` 无输出

## 实施记录

### 交付

- `internal/server/server.go`：`basicAuthMiddleware` 增加 variadic 受保护前缀；代理面传 `"/v1"`、管理面显式传 `"/api"`（不再依赖中间件的内部默认）；`NewProxyRouterWithAuth` 非 loopback 时挂同一份守卫
- `internal/server/proxy_handlers.go`：体量上限（`PI_SWITCH_MAX_BODY_BYTES`，缺省 32 MiB，非法值回退默认而非关闭上限）与其常量**放在唯一消费者旁边**；`c.Request.Body = http.MaxBytesReader(...)` 并保留单一读体点；超限返回 413 `{error:{type:"request_too_large",message:...}}`，早于任何上游调用/落库/计费
- `cmd/pi-switch/main.go`：抽出 `mustResolveBindAuth`，webui 直接/daemon 与 proxy 直接/daemon **四条路径全部经过它**；proxy 的 daemon 分支在 `daemon.Start` 之前校验；proxy 的 usage/顶层 help 补 `--generate-password`；`doctor` 输出生效的体量上限，并在该变量无法解析时给出警告
- 拒绝文案泛化为"共享的 management/proxy 密码"，因为同一条信息现在也覆盖 proxy 绑定

### Review 结果（Step ③：两个独立单轴 reviewer 并行）

两轴均**无 blocking**。已处理的 findings：

- **F9（Standards）**：`MaxProxyBodyBytes` 原先放进 kernel，但它只有一个消费者，与 `docs/architecture.md` 的 kernel 规则（被 2+ 域调用）及本票"改造面局限在唯一读体点"的范围声明都不一致 → 已移入 `proxy_handlers.go`、改为未导出，并加命名常量（学习自票 04 的同一类错误）
- **F2（Standards）**：413 测试的"0 条记录"此前是在**无路由 fixture** 上测的，而正向对照用的是另一个 fixture/router，两者差异不止 body 大小 → 已改为**同一 router、同一 fixture**：先测超限（413 + 上游命中 0 次 + 计数 0），再测正常请求（计数 1）。红检确认：临时移除 413 早返回后，断言以 `a capped request reached the upstream 1 time(s)` 失败
- **F1（Standards）**：原测试用 `POST /v1/models`（GET-only），命中的是 gin 的 no-route 链而非命名路由，属"重构即失效"的耦合 → 已改为 {method, path} 表，POST 用 `/v1/chat/completions`、GET 用 `/v1/models`
- **F5**：测试会读开发者真实的 `~/.pi/agent/sessions`（`conversationSource` 默认 sessionScan）→ 已在 `isolateConfig` 中隔离 `PI_AGENT_SESSIONS`
- **F3/F4**：缺省上限改用独立字面量 `33554432`；loopback 用例改为断言处理器自身的 502 `no_route` 信封，而不是"不等于 401/413"
- **F8**：管理面显式传 `"/api"`
- **F11**：`doctor` 现在报告生效上限，并对 `PI_SWITCH_MAX_BODY_BYTES=64MB` 这类值明确警告
- **F15**：`resolveBindAuth` → `mustResolveBindAuth`（该 helper 会终止进程）
- **F16**：拒绝文案与 proxy help/usage 对齐（错误信息本身是 D1 的文档载体）

未在本票修、已记录的后续项：

- **F14**：`POST /api/proxy/start`（`settings_handlers.go:115`）在派生后才失败，操作者要等约 15s 健康检查才拿到笼统的"failed health check"。安全不变量由子进程守卫保证（fail-closed），仅诊断体验差；属设置域表面，与票 04 记录的"200 但无工作"端点同批处理
- **F2（Spec 轴同名项）**：缺 `proxy start --host 0.0.0.0` 的自动化启动拒绝测试（`startProxy` 会 `os.Exit`，进程内不可测）。本票以真实二进制验证（exit 1 + 明确报错）作为证据，若要自动化需把守卫抽到可注入的层
- **F21（重要设计缺口，需产品决策）**：守卫只接受 HTTP Basic，而网关发布给本地代理的 provider 携带 `"apiKey": "pi-switch-proxy"`（`gateway.go:236`）、baseUrl 为 `<proxy>/v1`，Pi 会以 `Authorization: Bearer …` 发送 → **在具名 LAN 绑定下，网关配置的那个客户端无法通过认证**（通配绑定被改写为 127.0.0.1，掩盖了该问题）。两条出路：文档化"暴露代理需要支持 Basic 的客户端"，或在 spec 层重议"仅 Basic"。**明确不做**：把共享密码写进 `models.json`——那会把共享密钥发布到 `~/.pi/agent/models.json`

### 验证证据

- `go build ./...` 通过；`go vet ./...` 无输出；`gofmt -l internal/ cmd/` 无输出；`go test ./... -count=1` **16 包全绿**
- 真实运行（临时目录隔离，已清理）：非 loopback 无密码 → 退出 1 + 新文案；`doctor` 输出 `proxy body cap: 33554432 bytes`，对 `64MB` 明确警告；`proxy --help` 与顶层 help 均含 `[--generate-password]`
- 早前已记录的真实运行：带密码时无凭据 `/v1/chat/completions` 401（JSON + `WWW-Authenticate`，与上游 401 的 text/plain 区分）、正确凭据 200、`/healthz` 200、loopback `/v1/models` 200、超限 body 413 并给出字节上限

### 流程说明（必须记录）

两个 reviewer **都报告工作区在审查期间被修改**（我为处理 Spec 轴 findings 时改了 `server.go` 与测试）。这是我第三次犯"review 期间未冻结"的错误。Standards 报告虽仍按 md5 锚定并给出结论，但其行号可能对应不到最终代码；我已按最终 diff 重跑全部验证，并以修改后的测试做了红检。

### 另一处自我修正

实现中途我曾用"loopback 下 POST 返回 401"作为缺陷证据，实际那次是**上游 provider 的拒绝**（响应 text/plain、无 `WWW-Authenticate`），且端口上残留着前一个带密码的实例。已改为断言不触发上游的 `GET /v1/models`，并新增用例锁定"中间件的 401 是 JSON + 挑战头"这一区分。
