# pi-switch 架构评估报告

- **基准**：commit `46d8153`（2026-09-10）
- **范围**：`cmd/`、`internal/`、`webui/`、`extensions/`、`src/`、构建与分发
- **定位**：本文是评估结论与行动清单。架构导航以 `docs/architecture.md` 为准；边界不变量以 `docs/system-contract.md` 与 `docs/adr/` 为准，本文不重复定义。
- **复审**：完成行动项或触发 §5 条件时更新。

> **后续进展**：本文行动项已由 `docs/maintainability-plan.md` 的 ARCH-01～08 及其后的两轴 review 收敛项承接
> （HTTP 错误信封契约见 `docs/system-contract.md` §2.8，responsesMode / API 能力单一下发见 `internal/protocol`
> 与 `WEBUI_GUIDE.md`）。本文保留为 `46d8153` 基线的评估记录，其中的代码位置叙述以当时为准。

## 1. 结论

**不需要全面重构。** 分层（`server → 领域包 → config` 单向无环）、事实来源分离（config.json / models.json / requests.db）、协议转换注册表、显式发布边界、60+ Go 测试文件 + vitest/Playwright 覆盖，都是资产，重写会全部赔掉。

**不需要全面重构，需要精准收敛**（§3）：

- **P0 信任边界与诚实性（5 项，✅ 全部已完成 2026-09-11）**：A1 管理面认证缺口、A8 密码无生成路径、A9 Proxy 无认证且请求体无上限（一票：`.scratch/trust-boundary-and-honesty/issues/01`、`02`）、A2 CLI 假成功（`03`）、A7 服务端假成功 API 且 WebUI 已接线（`04`）。逐票证据、review 与验证见该 spec 的 `progress.md`。
- **P1 结构与工程（4 项，✅ 全部已完成）**：A3 handler 拆分、A4 retry 休眠标注、A10 保存失败静默吞掉、A11 CI Go 版本与 go.mod 不匹配（后四项一票：`.scratch/arch-review-eng-batch/issues/02`、`01`、`03`）。
- **P2 决策与卫生（6 项）**：A6 config 读取缓存（触发式）、A12 前端组件单体（观察）；A5 遗留 JS 归属与 A15 工作区残留✅ 已完成（`.scratch/arch-review-eng-batch` 后续，2026-09-11）；A13 ADR 编号重复与 A14 README 版本徽章漂移✅ 已完成（`.scratch/arch-review-eng-batch/issues/04`、`05`）。

**明确不做**见 §4。

## 2. 架构事实摘要

### 2.1 进程与入口

单个二进制，三个前端：

```text
CLI/TUI ──┐
WebUI ──→ Mgmt API :43110 (NewMgmtRouter) ──→ config/gateway/store/catalog/piagent
pi ─────→ Proxy  :43112 (NewProxyRouterWithAuth) ──→ config/translator/limit/store ──→ 上游
```

两个 router 独立，但**共用同一份认证实现**：`basicAuthMiddleware` 按路由前缀守护——管理面 `/api`，Proxy 面 `/v1`，健康探活（`/healthz`、`/health`）保持免认证以便诊断。两个 router 都由实际绑定地址（CLI `--host`，非 config 声明）决定是否启用守卫；非 loopback 且无共享密码时启动被拒绝（`ValidateBindAuth`）。Mgmt 另托管 `//go:embed` 的 WebUI 资源。daemon 模式自 spawn 同一可执行文件，用 PID 文件 + 端口 health + 进程 identity 判定存活（`internal/daemon`）。

### 2.2 依赖方向

```text
cmd/pi-switch/main.go                命令分发、daemon 开关（薄）
  └─ internal/server                 路由 + handler + 代理核心（厚）
       ├─ server.go                  kernel：路由注册、认证、静态资源、构建信息
       ├─ *_handlers.go              六个 handler 域文件（proxy/profile/gateway/
       │                             package/settings/stats）
       ├─ retry.go / outbound.go / legacy_log.go   同包横切文件（见 A4）
       ├─ config    供应商/渠道/模型/设置的事实来源
       ├─ gateway   config → models.json 的派生与显式发布
       ├─ translator 协议转换（请求/响应/SSE 流，注册表驱动）
       ├─ limit     上下文与 max tokens clamp
       ├─ store     SQLite 请求事实 + 旧 requests.log 迁移
       ├─ stats     只读聚合 + 对话归属
       ├─ catalog   models.dev 元数据 enrich
       ├─ piagent   pi 包与 session 扫描
       └─ daemon    进程生命周期
tui / webui                          同一 Go 核心的另外两个视图
```

领域包之间无环，不反向依赖 `server`。

### 2.3 不得在重构中破坏的设计决策

1. `config.json` 是 Supplier/Channel/Model 的唯一事实来源；`models.json` 是显式发布的派生视图，供应商变更不自动写入（system-contract §1、§2.3）。
2. 路由只认裸模型 ID；0 个匹配 → 502 `no_route`，多个匹配 → 502 `ambiguous`，不猜（`proxy_handlers.go:resolveRoute`）。
3. 协议转换集中在注册表（`translator/registry.go:PlanRequest`），不在各 handler 内重复实现。
4. 请求事实 immutable 写 SQLite（`store.go`），统计只做只读聚合（`stats/service.go`）。
5. 发布校验失败零写入（`gateway` 的 canonical plan + 原子 rename）。

## 3. 行动项

### A1 [P0/安全] 管理 API 认证缺口 —— ✅ 已完成（2026-09-11, `e3f9f0c`）

- **证据（修复前）**：`internal/server/server.go:authMiddleware`；非 loopback 且无密码文件时直接 `c.Next()`（原注释自述"keep tests simple"）；同文件 `isLoopback` 把 `0.0.0.0`/`::` 视为 loopback。
- **影响**：`pi-switch webui --host 0.0.0.0` 且未设置 `~/.pi-switch/webui_password`（或 `PI_SWITCH_WEBUI_PASSWORD`）时，`/api/config`、`/api/profiles/*` 等管理接口无认证可达。这是信任边界问题，优先级高于一切结构问题。
- **动作**：非 loopback + 无密码 → 拒绝启动，或生成随机密码并打印一次；`isLoopback` 只认 `127.0.0.1`/`localhost`/`::1`。若确实需要反向代理场景，用显式 `trustedProxies` 配置，不用通配地址。
- **完成判据**：新增测试——非 loopback host + 无密码文件时 `/api/config` 返回 401/403；`0.0.0.0` 不被判为 loopback。
- **完成情况**：认证决策改为由**实际绑定地址**在构造时传入（`NewMgmtRouterWithAuth`），中间件不再读 config。`IsLoopback` 收敛为 `127.0.0.1`/`localhost`/`::1`/`[::1]`，且**空 host 也判为非 loopback**——`--host ""` 会让 `Run(":port")` 绑定所有接口（实测 `net.Listen(":0")` → `[::]`），此前被读成"本地"从而 fail-open，这是修复过程中新发现的同类缺陷。上述判据的测试全部落地（`internal/server/auth_binding_test.go`）。
- **相关**：密码无生成路径是 A8（本项根因）；Proxy 完全无认证是 A9。三项同批完成。

### A2 [P0/诚实性] CLI 假成功命令 —— ✅ 已完成（2026-09-11, `b5275b5`）

- **证据（修复前）**：`cmd/pi-switch/main.go` — `handlePackage`（add/sync/show/delete 打印 `{"ok":true}` 不做事）、`handleCcs`（返回空数组）、`handlePresets`（恒空）、`handleStatsCLI`（仅提示用 webui）、`handleDoctor`（永远 `doctor: ok`）。
- **影响**：CLI/脚本按 stdout JSON 判断成功，实际什么都没发生。`package list/import` 已真实实现（`server.ListInstalledPackages` / `ImportPiPackages`），同命令族内不一致放大误导。
- **动作**：能接已有实现的接入；不能实现的改为 stderr 说明 + 非零退出（`not implemented`），不再打印成功 JSON。
- **完成判据**：每个命令至少一条测试断言"未实现时退出码非零且 stdout 不含 ok"。
- **完成情况**：`package add/show/delete` 接线到服务端真实逻辑（抽出 `AddInstalledPackage`/`GetInstalledPackage`/`DeleteInstalledPackage` 供 handler 与 CLI 共用）；`package sync`、`ccs import`、`stats` 改为 stderr 说明 + 退出码 2；`doctor` 改为真实探测（config 可读性、DB 可读、两个 daemon 状态、运行中的暴露监听器无密码）；`preset` 接线到真实目录（服务端一直返回 4 条静态预设，旧 CLI 打印 `[]`）。**`ccs list` 的空数组保留**——确无 CCS provider 存储，空集合未声称任何副作用。
- **后续（未纳入本项）**：README 的 CLI 段落仍有本项之外的失真——`package toggle`、`config backups`、`config export/import` 实测均返回非零（未实现或不存在）；`import ccswitch` 一节已于 2026-09-11 改为据实描述。
- **后续补齐（2026-09-11，`.scratch/provider-cli-wiring`）**：`provider` 一族当时也未处理——`--help` 宣布 8 个子命令，实际只识别 3 个，而服务端能力都是真实实现的。已接线 `add`/`duplicate`/`test`/`fetch-models`/`expose` 五个命令（另加 `--models` 与默认 `main` 渠道），help 与两个 README 的示例同步校正；机械核验 README 提到的 9 个子命令全部可识别。
- **相关**：服务端同族问题（HTTP API + WebUI）是 A7。

### A3 [P1/结构] `internal/server/server.go` 按 handler 领域拆文件 —— ✅ 已完成

- **证据（拆分前）**：3645 行、118 个函数；最近 50 个提交中 20 次改动该文件（40% 命中率）；`retry.go`/`outbound.go`/`legacy_log.go` 已证明同包拆文件可行。
- **影响**：评审成本与变更碰撞集中在 gateway/proxy/stats/packages 四类 handler 上。
- **动作**：**同 package 拆文件，零接口变更**；路由注册保留在 `server.go`。不换包、不引入 service 接口层。
- **完成情况**：`server.go` 收至 522 行 / 18 个函数，只剩 router、auth、静态资源、构建信息与 kernel helper（`configPath`、`saveConfig`、`resolveModelsDevProvider`、`sessionScanCandidates`、`conversationCandidates`）。六个域文件为 `proxy_handlers.go`（含 `handleChatCompletions`/`handleStream`/`resolveRoute`/`clampBody`）、`profile_handlers.go`、`gateway_handlers.go`、`package_handlers.go`、`settings_handlers.go`、`stats_handlers.go`。被 2+ 域调用的 helper 留在 kernel，域文件之间无内部依赖。
- **完成判据**：`server.go` 只剩 router/auth/静态资源/横切工具；测试文件一行不改仍全绿。二者均已满足。拆分方法、逐票证据与偏差裁决见 `.scratch/split-server-handlers/`。

### A4 [P1/可发现性] 标注 `retry.go` 为休眠原语 —— ✅ 已完成（2026-09-11）

- **证据**：`internal/server/retry.go` 顶部无休眠说明，读起来像生效中的策略引擎；`.scratch/remove-failover-chain/spec.md` D1 明确"保留原语与 `circuitBreaker` 占位，供后续 per-conversation 熔断复用"；生产路径只取单候选（`proxy_handlers.go:resolveRoute` 的 `candidates[0]`），`PUT /api/proxy/failover` 返回 410（`settings_handlers.go:handlePutFailover`）。
- **影响**：新读者误判 failover 可用；不清除上下文时也容易被误删或误改。
- **动作**：`retry.go` 文件头注释说明"当前休眠、代理路径为单候选直通、复用前提见 spec"；`docs/architecture.md` 补一句同一事实。
- **完成判据**：文件头注释 + 架构文档一句话，且 `retry.go` 无生产调用点的事实有出处可查。
- **完成情况**：文件头加了 DORMANT 说明，并**区分了两件事**——调度引擎（`expandAttempts`/`admitForRound`/`waitForRound`/`classifyUpstreamError` 等）在非测试代码中零调用，而校验函数（`validateRetryFields`/`validateSettingsRetry`）仍被 profile 与 settings handler 调用；说明同时给出 `remove-failover-chain/spec.md` D1 的出处与"不要期望此处改动影响生产、未经确认不要删除"的告警。`docs/architecture.md` 已补同一事实。

### A5 [P2/决策] 遗留 JS 层归属 —— ✅ 已完成（2026-09-11）

- **证据**：`package.json:32-35` 的 `pi.extensions` 指向 `./extensions/index.ts`，但 `files`（:9-14）不含 `extensions/` 与 `src/`，发布包中该入口不存在；该扩展 import `../src/commands.js`（旧 Node 实现，含空函数体）。
- **动作**：产品决策二选一——恢复（补 `files` 并接入 Go 核心理念的新实现）或摘除 `pi.extensions` 字段并归档 `extensions/`、`src/`。
- **完成判据**：npm 包内容与 manifest 一致，不存在悬空入口。
- **完成情况（用户裁决：摘除并归档）**：`package.json` 的 `pi.extensions` 字段已移除（该字段指向 `./extensions/index.ts`，而该路径从未出现在 `files` 中，属悬空入口）；`src/` 与 `extensions/` 用 `git mv` 移入 `legacy/` 并加 `legacy/README.md` 说明。**核验**：`npm pack --dry-run` 的包内条目里 `legacy/` 为 0，`files` 仍为 `bin/`、`webui/dist/`、两个 README；`scripts/build-go.sh` 与 `bin/pi-switch.js` 均不引用这两个目录，移动后 `go build`/`vet`/`go test` 全绿。
- **归档而非删除的理由**（已写入 `legacy/README.md`）：`legacy/src/sync.js` 含有真实的配置加密导出/导入实现，作用于同一个 `~/.pi-switch/config.json`——服务端对应能力目前是 501（见 A7），将来实现时应参考它。

### A6 [P2/触发式] config 读取缓存

- **证据**：每个 proxy 请求都 `config.LoadConfigAtPath`（如 `proxy_handlers.go:handleChatCompletions`）；管理面守卫（`basicAuthMiddleware`）也在每请求读一次 config。
- **动作**：现在不优化。出现可测的延迟/QPS 证据后，按 mtime 缓存 `LoadConfigAtPath`。触发条件见 §5。

### A7 [P0/诚实性] 服务端假成功 API 且 WebUI 已接线 —— ✅ 已完成（2026-09-11, `0138e3a`）

- **证据（修复前）**：`settings_handlers.go:handleConfigExportStub` 返回 `{"ok":true,"path":"/tmp/export.json"}`（该文件不存在），`handleConfigImportStub`、`handleConfigRestoreStub` 同为恒成功；`profile_handlers.go:handleCcsProviders`（恒空）与 `handleCcsImport`（恒 ok、imported=0）；`settings_handlers.go:handleInit` 恒 ok。`webui/src/api.ts` 真实调用这些端点（config export/import/restore、ccswitch、init）。
- **影响**：WebUI 用户点"导出/导入/恢复配置"或 CCS 导入会看到成功提示，实际无任何操作；与 A2 同族，但用户面更广、更易被当成可用功能。
- **动作**：与 A2 同一把尺子——能接已有实现的接上；不能实现的返回 501 `{error:{type:"not_implemented"}}`，并在 WebUI 隐藏或禁用对应入口。
- **完成判据**：每个 stub 要么有真实实现 + 测试，要么返回 501 且 WebUI 不再展示成功路径；不存在"200 ok 但无副作用"的端点。
- **完成情况**：config export/import/restore、`/api/backups`、ccswitch import、`/api/init` 六个端点改为 501；WebUI 侧删除了对应入口（Backups 面板改为"未实现"说明、删除 "Import from cc-switch" 弹窗与按钮、"Initialize config" 按钮改为说明），并清理了随之失效的客户端方法与解码器。**勘误**：本项原以为"导出/恢复可复用 `handleBackups` 的备份能力"，实际全仓没有任何备份实现（`handleBackups` 本身就是恒空数组），故无"接已有实现"的选项。
- **遗留项关闭情况（2026-09-11，`known-gaps-remediation`）**：本项最后一条判据此前在**全仓范围**不成立，三处已逐一处理：
  - `POST /api/proxy/start` 非 daemon 分支（**已修**）：原返回 `running:true, message:"proxy started (stub)"`，而该分支不派生进程、不监听端口；现返回 501 `{error:{type:"not_implemented",...}}` 并指明需 `daemon:true`（或 host/port）。
  - 管理 API 启动的失败延迟（**已修**）：原描述"约 15s"经实测为按机制计——15 次尝试 ×（`/healthz`+`/health` 各 500ms 超时 + 200ms 间隔）；端口被占用或探测超时时最坏约 18s（实测 18037ms），连接被拒时约 3s。现子进程一退出即结束等待（同场景实测 17ms）。**端口占用的判定范围要说清**：handler 不再按错误文本分类（改用 `daemon.ErrPortInUse` + `errors.Is`），但 `daemon.Start` 内部仍按子进程日志里的 `address already in use` 判定——日志是子进程的输出，没有类型可言，这是唯一可行的读法，故该处保留文本判定。
  - `gateway health` / `gateway start` 的恒定 payload（**部分修**）：`has_models_file` 此前是字面量 `true`，现真实核对 `gateway.ModelsPath()`；`upstreams_total` 本就真实。`POST /api/gateway/start` 与 `running`/`last_notify` 保持不变——网关的 `mode: "logical-isolation"` 表明它是"发布 models.json"这个逻辑概念，没有常驻进程也没有通知记录可查，故不谎报之外也无从改造。
- **判据结论（不要读成已满足）**：上面三处修完后，"不存在 200 ok 但无副作用的端点"这条**在全仓范围仍不成立**，因为 `POST /api/gateway/start` 依然返回 `running:true` 而不做任何事。它与 A7 其余条目性质不同：那六处是"没实现却报成功"，这一处是"设计上无动作可做"（网关无进程），因此本项选择保留端点并只在文档中说清，而不是改成 501。是否该把它从判据里排除，属该判据自身的表述问题。
- **F21（网关 published provider 无法鉴权）**：已按 **A 案**落地（`known-gaps-remediation` + `review-remediation/01`）——两条 publish 路径与非 loopback 配置下于成功响应带 `warnings`，CLI `gateway publish` 打到 stderr，两个 README 记录限制；不引入新凭据，不把共享密码写进 `models.json`。判定曾在两处判错面（管理监听的绑定地址 / 被通配改写的 baseUrl），现已改为以**配置里的代理 host** 为主、发布条目自身 baseUrl 为辅。**仍未关闭**：根治方案（可发布的代理专用 token）未决；且运行期 `proxy start --host X` 与配置不一致时仍可能漏判（正确来源应是运行中 daemon 的 host）。
- **相关**：A2（CLI 同族）。

### A8 [P0/安全] WebUI 密码无生成路径（A1 的根因） —— ✅ 已完成（2026-09-11, `e3f9f0c`）

- **证据（修复前）**：`server.go:resolveWebUIPassword` 只读 `PI_SWITCH_WEBUI_PASSWORD` 与 `~/.pi-switch/webui_password`；全仓无任何写该文件的代码（仅 `webUIPasswordPath` 的路径拼接）。
- **影响**：非 loopback 时"无密码就放行"不是边缘情况而是默认状态——没有任何机制能产生密码文件，管理面等于永久无认证。
- **动作**：启动时若绑定非 loopback 且无密码：生成随机密码写入用户私有文件（0600）并打印一次；或直接拒绝启动。二选一，需与 A1 同批实现。
- **完成判据**：全新环境非 loopback 启动后，认证要么强制生效（密码文件存在且权限 0600），要么启动被拒绝；测试覆盖所选路径。
- **完成情况**：两者都实现——**默认 fail-closed**（非 loopback 且无密码则拒绝启动），`--generate-password` 是显式选项（生成随机密码写入 `webUIPasswordPath()` 0600 并只打印一次）。凭据优先级定为 **generate > 密码文件 > `PI_SWITCH_WEBUI_PASSWORD`**，使"打印出来的密码"与 daemon 子进程实际强制的密码必然一致。实测该文件权限为 `-rw-------`。

### A9 [P0/安全] Proxy 无认证且请求体无上限 —— ✅ 已完成（2026-09-11, `e20e9ac`）

- **证据（修复前）**：`server.go:NewProxyRouter` 仅挂 `gin.Recovery()`，无认证中间件；`proxy_handlers.go:handleChatCompletions` 以 `io.ReadAll(c.Request.Body)` 读取请求体，无 `http.MaxBytesReader`。
- **影响**：`pi-switch proxy --host 0.0.0.0` 时局域网可无认证消耗付费上游额度，并可用超大 body 打爆内存。默认绑 loopback 只降低触发概率，不构成防护。
- **动作**：非 loopback 绑定时要求同一套 Basic 认证（复用 A1/A8 的密码）；proxy 入口加 `http.MaxBytesReader`（上限可配，缺省建议 32MB）。若产品明确不支持远程代理，则非 loopback 直接拒绝启动并在文档说明。
- **完成判据**：非 loopback + 无密码时 proxy 不可用；超大 body 返回 413；测试覆盖两条路径。
- **完成情况**：非 loopback 时 Proxy 挂上与管理面**同一份** Basic 守卫（同一实现、同一密码来源），只守护 `/v1` 前缀；请求体加 `http.MaxBytesReader`（`PI_SWITCH_MAX_BODY_BYTES`，缺省 32 MiB，非法值回退默认而非关闭上限），超限返回 413 且早于任何上游调用/落库/计费。CLI 的 webui/proxy 共四条启动路径全部经过同一守卫，daemon 子进程独立重新校验。
- **已知缺口（需产品决策）**：守卫只接受 HTTP Basic，而网关发布给本地代理的 provider 携带 `apiKey: "pi-switch-proxy"`，客户端会以 `Authorization: Bearer …` 发送——**在具名 LAN 绑定下，网关配置的那个客户端无法通过认证**（通配绑定会被改写为 127.0.0.1，掩盖该问题）。出路是文档化"暴露代理需要支持 Basic 的客户端"，或在 spec 层重议"仅 Basic"；**不可**把共享密码写进 `models.json`。

### A10 [P1/诚实性] 保存失败静默吞掉 —— ✅ 已完成（2026-09-11）

- **证据**：`_ = saveConfig(cfg)` 8 处（`profile_handlers.go` 5 处、`settings_handlers.go:handlePutSettings` 1 处、`cmd/pi-switch/main.go` 2 处），随后仍返回 `200 {"ok":true}`。
- **影响**：磁盘满、权限错误时配置写入丢失，但所有入口报告成功——用户配置静默丢失。
- **动作**：`saveConfig` 失败必须让 handler 返回 5xx 与错误信息；CLI 路径退出码非零。
- **完成判据**：写盘失败注入测试（只读目录或 mock）返回 5xx / 非零退出；全仓 `_ = saveConfig` 归零。
- **完成情况**：六个服务端调用点（`handlePutSettings` + profile 五处）改为写盘失败返回 500，CLI 的 `provider use`/`provider delete` 改为非零退出且不打印成功文案；全仓 `_ = saveConfig` 已归零。失败注入用"配置文件可读但所在目录不可写"，并配了可写路径的正向对照。红检确认：恢复吞错后断言以 `put settings = 200 ({"ok":true}), want 500` 失败。

### A11 [P1/工程] CI Go 版本与 go.mod 不匹配 —— ✅ 已完成（2026-09-11）

- **证据**：`.github/workflows/ci.yml:36,150,193` 为 `go-version: "1.23"`，`go.mod:3` 为 `go 1.24.2`；当前依赖 `GOTOOLCHAIN=auto` 隐式下载工具链。
- **影响**：离线/受限镜像或设 `GOTOOLCHAIN=local` 时 CI 直接失败；构建时间与网络依赖隐性存在。
- **动作**：CI 三个 job 的 go-version 对齐 `go.mod`（或显式声明 `GOTOOLCHAIN` 策略）。
- **完成判据**：CI 不触发工具链下载即可通过；版本以 `go.mod` 为单一事实来源。
- **完成情况**：三处 `go-version: "1.23"` 改为 `go-version-file: go.mod`（setup-go 直接读 `go 1.24.2`）。选择读文件而非改常量，正是为了消除"人工同步版本"这一漂移来源。CI 实跑需推送后才能确认。

### A12 [P2/观察] 前端组件单体

- **证据**：`webui/src/components/ProfilesPanel.tsx` 49KB、`StatsPanel.tsx` 47KB（`StatsPanel.test.tsx` 62KB）、`GatewayPanel.test.tsx` 33KB；最近 50 提交中 `GatewayPanel.tsx` 与其测试各被改 10 次。
- **影响**：churn × 尺寸的组合与 A3 前的 `server.go` 同构，评审与冲突成本会持续上升。
- **动作**：现在不动。A3 完成后按同一把尺子观察 1–2 个迭代；若 GatewayPanel 继续高频变更，再立拆分 spec（不预先设计）。
- **完成判据**：无（观察项）；触发条件见 §5。
- **相关**：A3（同构问题，方法可复用）。

### A13 [P2/文档] ADR 编号重复 —— ✅ 已完成（2026-09-11）

- **证据（修复前）**：`docs/adr/0004-responses-provider-passthrough.md` 与 `docs/adr/0004-supplier-side-pi-session-scan.md` 并存。
- **影响**：引用 "ADR 0004" 有歧义；后续编号连续性被破坏。
- **动作**：给其中一个重新编号（按时间保持单调），并更新相关文档/`.scratch` 引用；不改内容。
- **完成判据**：`docs/adr/` 编号唯一且连续。
- **完成情况**：把 `supplier-side-pi-session-scan` 重编为 `0012`，即"取当前最大编号 +1"（ADR-FORMAT.md 的规则），文件内标题同步为 `ADR-0012`、内容零改动。**不声称编号与时间全局单调**：该 ADR 原始引入于 2026-08-31，早于 0007–0011（09-02…09-08），若真按时间重排就需改动 5 个文件及其引用，代价与收益不成比例，故选择追加。`.scratch` 下的历史 spec 按惯例不回改，其中指向该 ADR 的编号引用已成为历史记录。

### A14 [P2/文档] README 版本徽章漂移 —— ✅ 已完成（2026-09-11）

- **证据**：`README.md` 徽章为 `20260902.0.0`，`package.json` 为 `20260910.0.2`。
- **动作**：发布流程同步徽章，或改为动态 release badge。
- **完成判据**：两者一致，或不一致会被 CI 检出。
- **完成情况**：两个 README 的徽章从 `20260902.0.0` 更新为 `20260910.0.2`，与 `package.json` 一致。"不一致会被 CI 检出"未实现，可另立项。

### A15 [P2/卫生] 工作区残留 —— ✅ 已完成（2026-09-11，经用户确认）

- **证据**：`target/` 6.9G（ADR 0007 已宣告 Go-only）、`.gocache/` 303M；均被 gitignore。`bin/` 101M 为本地构建产物（仅 `bin/pi-switch.js` 被跟踪），不需处理。
- **影响**：磁盘占用与工具链认知噪声；新 agent 可能误在 Rust 残留目录中探索。
- **动作**：删除 `target/` 与 `.gocache/`（属破坏性操作，需用户确认后执行）；`.gitignore` 已覆盖，无需改动。
- **完成判据**：目录不存在，且 `git status` 不受影响。
- **完成情况**：删除 `target/`（6.9G，Rust 残留）与 `.gocache/`（303M）。删除前核实二者均被 gitignore、**0 个被跟踪文件**，`target/` 内容为 CACHEDIR.TAG/debug/release/napi-rs 等纯构建产物。删除后 `git status` 干净。**注意**：`df` 的空闲空间读数未出现可见变化（同一挂载点其他写入活动掩盖了差值），故"释放 7.2G"未能从 df 证实，仅以目录消失为凭。
- **相关**：ADR 0007。

## 4. 明确不做

1. 不重写、不换 gin / bubbletea / SQLite 技术栈。
2. 不拆进程/服务——proxy 与 mgmt 共二进制是分发优势（npm shim + 6 目标交叉编译）。
3. 不给领域包加接口抽象层；保持 `server → 领域包 → config` 单向无环。
4. 不"接线" `retry.go`——per-conversation 熔断是新功能，按 spec 单独立项；混进本轮收敛会扩大爆炸半径。
5. 不做无证据的批量重命名/移动；不动 `docs/system-contract.md` 的不变量。
6. 不提前做 config 缓存、并发优化、性能调优。

## 5. 复评触发条件

- A3 完成后 `server.go` 仍随每次改动被踩踏 → 继续按 handler 领域拆。
- 出现独立复用代理核心的第二消费方 → 才评估把 proxy core 从 `server` 包提升为独立包。
- 有 QPS/延迟数据证明 config 读取是热点 → 启动 A6。
- per-conversation 熔断立项 → 基于 A4 的 spec 恢复 retry 原语或删除。
- 认证模型变化（远程访问、多用户）→ 重估 A1/A8/A9 方案。
- `GatewayPanel`/`StatsPanel` 的 churn 或尺寸继续上升 → 启动 A12 的拆分 spec。
- A2/A7 任一实现落地 → 同批处理 A10（保存错误传播），避免再次出现"成功但无副作用"。
