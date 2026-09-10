# 11: 文档同步到新结构

**What to build:** 拆分与收敛落地后，版本化文档仍指向正确的文件与符号，导航不失效。端到端行为：读者按 `docs/architecture.md` 的入口表能找到实际存在的文件；`docs/architecture-review.md` 中行号类证据改为符号引用后不会随代码漂移而失效，A3 条目显示为已完成。

**实现要点**（spec D9）：

- `docs/architecture.md`：入口表与调用链的文件路径改为新文件（六域文件 + kernel）
- `docs/architecture-review.md`：A3 标记完成；行号类证据改为符号引用
- `.scratch` 下的历史 spec 不回改

**Blocked by:** 01: proxy 域 handler 迁出 kernel；02: profile 域 handler 迁出 kernel；03: gateway 域 handler 迁出 kernel；04: package 域 handler 迁出 kernel；05: settings 域 handler 迁出 kernel；06: stats 域 handler 迁出 kernel；07: kernel 提炼与残余审计；08: config 路径解析与原子写收敛；09: 成本计算单一实现

**Status:** resolved (2026-09-11)

- [x] `docs/architecture.md` 入口表与调用链中的每个文件路径都真实存在，且指向拆分后的新文件
- [x] `docs/architecture-review.md` 的 A3 标记为完成，且不再以行号作为证据（改为符号引用）
- [x] 文档中出现的符号名在代码中可检索到（无搬迁后的失效符号名）
- [x] `.scratch` 历史 spec 零改动
- [x] 本票不新增 ADR、不修改 `CONTEXT.md`（spec Further Notes：拆分是实现结构，非领域词汇）

## 实施记录

- `docs/architecture.md`：
  - 入口表两行改为指向新文件——WebUI 管理 API 的「继续阅读」列出五个域文件，Proxy 入口写 `proxy_handlers.go:handleChatCompletions`、`handleStream`
  - 「Proxy 入口对应代码」补 kernel + 六域文件结构图与「2+ 域 helper 留 kernel、域文件互不调用」的规则，并把 `BuildOutboundRequest`/`PlanRequest` 从 `server.go` 块中移出到各自真实文件（`internal/server/outbound.go`、`internal/translator/translator.go`）
  - WebUI 调用链里的 `internal/server/server.go` 补注「kernel：NewMgmtRouter 注册 /api/*」
- `docs/architecture-review.md`（该文件此前未纳入版本控制，本票一并提交）：
  - A3 标题标 `✅ 已完成`，把原「动作」里过时的文件名（`handler_gateway.go` 等）改为最终落地的六个文件名，新增「完成情况」段记录 `server.go` 523 → 实测 522 行 / 18 个函数与 kernel 残余，指向 `.scratch/split-server-handlers/` 取逐票证据
  - 行号型证据全部改为符号引用：A1（`server.go:authMiddleware`/`isLoopback`）、§2.3 第 2 条（`proxy_handlers.go:resolveRoute`）、A2（`cmd/pi-switch/main.go` 的五个命令符号，去掉 `:355` 等行号）、A4（`proxy_handlers.go:resolveRoute` 的单候选 + `settings_handlers.go:handlePutFailover` 的 410，取代 `server.go:2986`/`:3213`）、A6（`proxy_handlers.go:handleChatCompletions` 取代 `server.go:2950 附近`）、§2.2 拓扑图两处 router 行号
  - §2.2 依赖方向补上 `server.go` kernel、`*_handlers.go` 六域与三个横切文件三行
- 符号可检索性已逐条机械校验：文档出现的 `resolveRoute`、`clampBody`、`handleChatCompletions`、`handleStream`、`handlePutFailover`、`authMiddleware`、`isLoopback`、`NewMgmtRouter`、`NewProxyRouter` 全部落在真实文件；文档涉及的文件路径 9/9 存在
- 未回改 `.scratch` 下历史 spec；未新增 ADR；`CONTEXT.md` 未改
- 说明：`docs/architecture.md` 工作区另有一处非本票改动（顶部指向 `architecture-review.md`/`system-contract.md` 的交叉引用行），同属本次文档同步的导航修正，一并提交
