# pi-switch 架构评估报告

- **基准**：commit `46d8153`（2026-09-10）
- **范围**：`cmd/`、`internal/`、`webui/`、`extensions/`、`src/`、构建与分发
- **定位**：本文是评估结论与行动清单。架构导航以 `docs/architecture.md` 为准；边界不变量以 `docs/system-contract.md` 与 `docs/adr/` 为准，本文不重复定义。
- **复审**：完成行动项或触发 §5 条件时更新。

## 1. 结论

**不需要全面重构。** 分层（`server → 领域包 → config` 单向无环）、事实来源分离（config.json / models.json / requests.db）、协议转换注册表、显式发布边界、60+ Go 测试文件 + vitest/Playwright 覆盖，都是资产，重写会全部赔掉。

**需要 4 项精准收敛**（§3）：2 个信任边界/诚实性 bug（A1、A2），2 个结构卫生（A3、A4）；另有 1 个产品决策（A5）和 1 个触发式优化（A6）。

**明确不做**见 §4。

## 2. 架构事实摘要

### 2.1 进程与入口

单个二进制，三个前端：

```text
CLI/TUI ──┐
WebUI ──→ Mgmt API :43110 (NewMgmtRouter) ──→ config/gateway/store/catalog/piagent
pi ─────→ Proxy  :43112 (NewProxyRouter)  ──→ config/translator/limit/store ──→ 上游
```

两个 router 独立：Proxy 无认证中间件（默认只绑 loopback）；Mgmt 挂 `authMiddleware` 并托管 `//go:embed` 的 WebUI 资源。daemon 模式自 spawn 同一可执行文件，用 PID 文件 + 端口 health + 进程 identity 判定存活（`internal/daemon`）。

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

### A1 [P0/安全] 管理 API 认证缺口

- **证据**：`internal/server/server.go:authMiddleware`；非 loopback 且无密码文件时直接 `c.Next()`（原注释自述"keep tests simple"）；同文件 `isLoopback` 把 `0.0.0.0`/`::` 视为 loopback。
- **影响**：`pi-switch webui --host 0.0.0.0` 且未设置 `~/.pi-switch/webui_password`（或 `PI_SWITCH_WEBUI_PASSWORD`）时，`/api/config`、`/api/profiles/*` 等管理接口无认证可达。这是信任边界问题，优先级高于一切结构问题。
- **动作**：非 loopback + 无密码 → 拒绝启动，或生成随机密码并打印一次；`isLoopback` 只认 `127.0.0.1`/`localhost`/`::1`。若确实需要反向代理场景，用显式 `trustedProxies` 配置，不用通配地址。
- **完成判据**：新增测试——非 loopback host + 无密码文件时 `/api/config` 返回 401/403；`0.0.0.0` 不被判为 loopback。

### A2 [P0/诚实性] CLI 假成功命令

- **证据**：`cmd/pi-switch/main.go` — `handlePackage`（add/sync/show/delete 打印 `{"ok":true}` 不做事）、`handleCcs`（返回空数组）、`handlePresets`（恒空）、`handleStatsCLI`（仅提示用 webui）、`handleDoctor`（永远 `doctor: ok`）。
- **影响**：CLI/脚本按 stdout JSON 判断成功，实际什么都没发生。`package list/import` 已真实实现（`server.ListInstalledPackages` / `ImportPiPackages`），同命令族内不一致放大误导。
- **动作**：能接已有实现的接入；不能实现的改为 stderr 说明 + 非零退出（`not implemented`），不再打印成功 JSON。
- **完成判据**：每个命令至少一条测试断言"未实现时退出码非零且 stdout 不含 ok"。

### A3 [P1/结构] `internal/server/server.go` 按 handler 领域拆文件 —— ✅ 已完成

- **证据（拆分前）**：3645 行、118 个函数；最近 50 个提交中 20 次改动该文件（40% 命中率）；`retry.go`/`outbound.go`/`legacy_log.go` 已证明同包拆文件可行。
- **影响**：评审成本与变更碰撞集中在 gateway/proxy/stats/packages 四类 handler 上。
- **动作**：**同 package 拆文件，零接口变更**；路由注册保留在 `server.go`。不换包、不引入 service 接口层。
- **完成情况**：`server.go` 收至 522 行 / 18 个函数，只剩 router、auth、静态资源、构建信息与 kernel helper（`configPath`、`saveConfig`、`resolveModelsDevProvider`、`sessionScanCandidates`、`conversationCandidates`）。六个域文件为 `proxy_handlers.go`（含 `handleChatCompletions`/`handleStream`/`resolveRoute`/`clampBody`）、`profile_handlers.go`、`gateway_handlers.go`、`package_handlers.go`、`settings_handlers.go`、`stats_handlers.go`。被 2+ 域调用的 helper 留在 kernel，域文件之间无内部依赖。
- **完成判据**：`server.go` 只剩 router/auth/静态资源/横切工具；测试文件一行不改仍全绿。二者均已满足。拆分方法、逐票证据与偏差裁决见 `.scratch/split-server-handlers/`。

### A4 [P1/可发现性] 标注 `retry.go` 为休眠原语

- **证据**：`internal/server/retry.go` 顶部无休眠说明，读起来像生效中的策略引擎；`.scratch/remove-failover-chain/spec.md` D1 明确"保留原语与 `circuitBreaker` 占位，供后续 per-conversation 熔断复用"；生产路径只取单候选（`proxy_handlers.go:resolveRoute` 的 `candidates[0]`），`PUT /api/proxy/failover` 返回 410（`settings_handlers.go:handlePutFailover`）。
- **影响**：新读者误判 failover 可用；不清除上下文时也容易被误删或误改。
- **动作**：`retry.go` 文件头注释说明"当前休眠、代理路径为单候选直通、复用前提见 spec"；`docs/architecture.md` 补一句同一事实。
- **完成判据**：文件头注释 + 架构文档一句话，且 `retry.go` 无生产调用点的事实有出处可查。

### A5 [P2/决策] 遗留 JS 层归属

- **证据**：`package.json:32-35` 的 `pi.extensions` 指向 `./extensions/index.ts`，但 `files`（:9-14）不含 `extensions/` 与 `src/`，发布包中该入口不存在；该扩展 import `../src/commands.js`（旧 Node 实现，含空函数体）。
- **动作**：产品决策二选一——恢复（补 `files` 并接入 Go 核心理念的新实现）或摘除 `pi.extensions` 字段并归档 `extensions/`、`src/`。
- **完成判据**：npm 包内容与 manifest 一致，不存在悬空入口。

### A6 [P2/触发式] config 读取缓存

- **证据**：每个 proxy 请求都 `config.LoadConfigAtPath`（如 `proxy_handlers.go:handleChatCompletions`）；`authMiddleware` 也每请求读一次。
- **动作**：现在不优化。出现可测的延迟/QPS 证据后，按 mtime 缓存 `LoadConfigAtPath`。触发条件见 §5。

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
- 认证模型变化（远程访问、多用户）→ 重估 A1 方案。
