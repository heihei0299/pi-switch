# Spec: 移除故障转移链（过渡清理，为 per-conversation 熔断让路）

Status: ready-for-agent
Version: 1.0
Author: pi-switch maintainers
Date: 2026-09-03
Scope: `config` / `server` 代理路由 / `webui` / `tui` / `docs` / `CONTEXT.md`（不含新熔断实现）

## 1. 背景与目标

### 1.1 问题
- 现有 `settings.proxy.failover: string[]` 为**进程级全局候选池**，配合 `retry.go` 的 `cooldownUntil[profile+baseUrl]` 全局冷却与 `expandAttempts` 多轮重试：单会话触发 `429`（`continue-and-cooldown`）会将对应 `profile/baseUrl` 置为全局冷却 60s，导致**所有会话**后续请求被迫进入同一 failover 链重试/等待，最终级联挂死。
- `429` 本是 per-credential 限流，却被放大为 per-process 熔断；且跨供应商隐式跳转导致“不知道打到哪、费用归谁”不可审计。

### 1.2 目标（本次）
- **切断跨会话污染**：移除跨供应商的全局故障转移链，使单会话 `429/5xx/transport` 不再影响其他会话。
- **为新熔断让路**：本次为最小切，保留 `retry.go` 原语与 `circuitBreaker` 占位，供后续 `per-conversation` 熔断复用；本次不引入新熔断逻辑。
- **不落 ADR**：本次为过渡清理，不沉淀永久决策；待新熔断设计时一并 ADR。

### 1.3 非目标
- 不在本 spec 实现新的 per-conversation 熔断/限流/重试策略。
- 不改变 `Supplier/Channel` 数据模型（多 channel + weight + 渠道 pin 保留）、`Gateway` 聚合与发布、`token/cost` 统计、`sessionScan` 归属。

## 2. 术语（以 CONTEXT.md 为准）
- **供应商（Supplier）**：`ProviderProfile`，为模型与凭证唯一事实来源。
- **渠道（Channel）**：`Upstream` 同义词，`name` 为供应商内唯一主键。
- **裸模型 ID**：如 `gpt-4o`，不含 `supplier/` 前缀。
- **渠道精确路由**：`supplier/channel/model` 精确 pin 到单渠道，不跨供应商。

本次后 `故障转移（Failover）` 为历史术语，不再作为功能术语使用（CONTEXT.md 首段已移除“支持同模型 failover”句）。

## 3. 决策（与 Q11-Q14 一致）

| 编号 | 决策 | 说明 |
|------|------|------|
| D1 | 仅删跨供应商 failover 链，最小切 | 删 `Failover` 字段与聚合，不删 `retry.go` 原语实现 |
| D2 | 后续熔断按 per-conversation 隔离 | 本次仅不破坏复用路径，不预埋实现 |
| D3 | 过渡期 429/5xx 直通透传 | 不重试、不冷却、不跳候选 |
| D4 | 存量配置静默忽略、API 410 | 首次保存删除字段 |

## 4. 功能规格

### 4.1 配置（config）
- **移除字段**：`Settings.Proxy.Failover []string`（`json:"failover,omitempty"`）。
- **兼容**：
  - 读：若 `config.json` 含 `failover`，解析时忽略（不报错），内存中不保留。
  - 写：`MigratedForSave` / 任意保存路径首次落盘时删除该键（若存在）。
  - 校验：`health` / `failover profile not found` warning 移除。
- **保留**：`Settings.Proxy.CircuitBreaker`、`RequestRetry`、`MaxRetryCredentials`、`MaxRetryInterval`、`DisableCooling`、`TransientErrorCooldownSeconds`、`RequestScopedErrors` 字段与类型保留不动（供新熔断复用），但过渡期不生效（见 4.2）。

### 4.2 代理路由（server）
- **路由解析 `resolveRoute(cfg, requested) -> (candidates, realModel, pinnedChannel)` 重定义**：
  ```go
  // 三段 pin：supplier/channel/model → 单候选，不跨供应商（保持现状）
  if strings.Count(requested, "/") >= 2 { supplier/channel/model → [supplier] }
  // 二段 pin：supplier/model → 单候选 [supplier]，不再追加 failover 补位
  if strings.Contains(requested, "/") { prefix==supplier && exposes → [prefix] }
  // 裸模型：仅走 current 指向的供应商；若 current 缺失或该供应商未暴露该模型 → 空候选 → 502 no_route
  // 不再遍历 failover 列表，不再全量扫描所有供应商
  ```
  - `candidates` 长度恒为 `0` 或 `1`（三段/二段 pin 成功为 1；裸模型命中 current 且暴露为 1；否则 0）。
  - `pinnedChannel` 语义保持（仅三段 pin 有值）。
- **转发语义（`handleChatCompletions` / `handleStream` / `handleResponses` 等）**：
  - 单候选单次直通：取 `candidates[0]` 对应 profile 的首选渠道（`PrimaryBaseURL` / `orderedChannels` 首元素，weight 排序仍用于选首渠，但不再用于“失败换渠道”）。
  - 不再调用 `expandAttempts` / `pinChannelAttempts` / `admitForRound` / `waitForRound` / `isCooling` / `coolForAttempt` / `candidateCooldownKeys` / `coolingHint`；失败分支直接透传原 `status` 与 `body`（含 429/5xx/transport），不写 `cooldownUntil`。
  - 不再返回 `failover_exhausted` 类型；统一为普通 upstream 错误透传（`429` 保持 `429`，`5xx` 保持 `5xx`，`502 no_route` 保持）。
  - 日志：`attempts` 相关聚合移除，单次请求日志仅记录单候选信息。
- **保留但休眠**：`retry.go` 文件与所有函数/类型保留，不删除；`server.go` 中不再调用它们（以注释 `// retained for per-conversation breaker, not used in transitional passthrough` 标记休眠调用点）。

### 4.3 API
- `PUT /proxy/failover`：改为 `410 Gone`，body `{ error: { message: "failover removed, will be replaced by per-conversation breaker", type: "gone" } }`。
- `GET /health` / `GET /proxy/status` 等回显中移除 `failover` 字段。
- 其他代理 API（`/v1/chat/completions`、`/v1/responses`、`/v1/messages`、streaming）行为按 4.2 直通。

### 4.4 WebUI
- `ProxyPanel.tsx`：移除 `FailoverEditor` 组件及其 `Card`、`api.setFailover` 调用、`mutateAfterFailover`。
- `ProxyPanel` 仅保留 Proxy host/port 与启停；`GatewayPanel` 不受影响。
- `types.ts`：`Settings.proxy.failover: string[]` 移除（或置为可选兼容字段但 UI 不展示）。
- `api.ts`：`setFailover` 保留为 deprecated stub（调用即 410）或直接移除（选移除，API 已 410）。
- `i18n`：移除 `Failover chain / No failover configured / failover` 相关 key（保留翻译文件兼容回退）。

### 4.5 TUI / CLI
- `Settings → Failover` 菜单项移除；`pi-switch proxy failover <p1,p2>` 命令改为提示 `failover removed` 并以非 0 退出或 410 透传。
- `cmd/pi-switch/main.go:handleProxy` 等 CLI 帮助文本移除 failover 示例。

### 4.6 文档
- `README.md / README_ZH.md`：移除 `proxy failover` 命令示例、Gateway Routing & Failover 章节中的 failover 描述、WebUI 中 Proxy/Gateway 面板编辑故障转移链的说明；保留“渠道精确路由 `supplier/channel/model` 精确 pin”句。
- `CONTEXT.md`：首段已移除“支持同模型 failover”句（本 spec 已完成）。

### 4.7 兼容与迁移
- 旧 `config.json` 含 `failover` 的用户：升级后首次任意保存（或代理启动时 `MigratedForSave`）自动删除该键，无需手动迁移；读时不报错。
- 依赖 `failover_exhausted` 错误类型的脚本：改为处理普通 upstream status 透传（429/5xx 原样）。
- 裸 `model` 曾依赖全量扫描的用户：改为显式 `supplier/model` 或设置 `current` 指向期望供应商。

## 5. 非功能与约束
- 单请求单供应商可审计：日志与统计中的 `provider` 恒为单值，不再出现多候选 `attempts` 数组。
- 故障隔离：单会话 429 不写全局状态，不影响其他会话后续请求。
- 为新熔断保留复用路径：`retry.go` 与 `CircuitBreaker` 类型不删，避免未来重命名/迁移成本。

## 6. 验收标准（Acceptance Criteria）

1. **配置**：含 `failover` 的旧 `config.json` 可正常加载，内存中无 `Failover` 残留，首次保存后文件不含 `failover` 键。
2. **API**：`PUT /proxy/failover` 返回 `410` 且 body 含 `gone`；`GET /health` 不再含 `failover`。
3. **路由**：
   - `POST /v1/chat/completions` with `model="supplier/channel/model"` → 仅打该渠道，失败透传原 status，不跳候选。
   - `model="supplier/model"` → 仅打该供应商首渠，失败透传，不追加 failover。
   - `model="bare-id"` → 仅当 `current` 供应商暴露该模型时成功，否则 `502 no_route`（不再全量扫描）。
4. **429 隔离**：会话 A 对供应商 X 触发 429 后，会话 B 对同一供应商 X（或不同供应商）的下一请求不受影响（不进入 failover、不被冷却阻塞），可复现验证。
5. **直通**：`429/500/502/503/504/transport` 均直通原 status/body，不写 `cooldownUntil`，不等待。
6. **UI**：WebUI Proxy 面板无 FailoverEditor；TUI 无 Failover 菜单；CLI `proxy failover` 提示已移除。
7. **类型**：`go test ./...` 与 `NODE_ENV=test npx vitest run` 通过；`retry.go` 保留但无调用，单测中与 failover 相关的多候选断言更新为单候选。

## 7. 范围外
- 新 per-conversation 熔断/限流/重试策略（另起 spec）。
- 多 channel 权重“失败换 channel”语义（本次权重仅用于选首渠，不用于重试）。

## 8. 风险与回滚
- 风险：裸 `model` 用户需改为显式 pin，否则 `no_route`；需在 Release Notes 中显式提示。
- 回滚：回滚即恢复 `Failover` 字段与 `resolveRoute` 多候选聚合；因本次未删 `retry.go` 实现，回滚成本低。

## 9. 依赖与顺序
- `config` 字段移除 → `server` 路由直通 → `webui/tui/docs` 清理 → 单测/快照更新。

## 10. 参考
- 现状实现：`internal/config/config.go:Settings.Proxy.Failover`、`internal/server/server.go:resolveRoute/handleChatCompletions/handleStream`、`internal/server/retry.go:expandAttempts/coolForAttempt`、`webui/src/components/ProxyPanel.tsx:FailoverEditor`。
- 关联 ADR：`0006 供应商与网关彻底逻辑独立`、`0008 渠道分区模型`（本次不改其模型，仅改路由聚合）。
