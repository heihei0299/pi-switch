# pi-switch 可维护性落实审查报告

- 审查基准：`main @ 6aee77ae`
- 日期：2026-09-11
- 审查方式：静态代码审查；未重新执行 build / test / lint / typecheck
- 对照文档：
  - `docs/maintainability-spec.md`
  - `docs/maintainability-plan.md`

## 结论

ARCH-01～08 的主体目标已经落实，整体架构方向正确，且没有引入明显过度设计。

当前评价：约 **9/10**。

主要成果：

- Config 读写边界已收敛。
- Gateway Generated flow 已统一。
- TUI Stats 已去除直接 SQL。
- Profile 共享业务已移出 server。
- Protocol / responsesMode 已形成单一事实源。
- 未引入 `internal/app`、Repository 层、DI 或无必要 interface。

当前仍有 **1 个优先修复问题 + 4 个收尾项**。完成后建议停止继续扩大架构重构范围。

## 落实状态

| 项目 | 状态 | 结论 |
|---|---|---|
| ARCH-01 Config 严格读取 | ✅ | 已完成 |
| ARCH-02 Config 统一安全写入 | ✅ | 已完成 |
| ARCH-03 Generated / Draft 显式语义 | ⚠️ | Core 正确，但 Draft HTTP 路径仍有语义破坏 |
| ARCH-04 Gateway 三入口统一 | ✅ | 主路径已统一 |
| ARCH-05 Gateway Enrich 统一 | ⚠️ | Generated 正确，Draft enrich 策略需修正 |
| ARCH-06 TUI Stats 去 SQL | ✅ | 已完成 |
| ARCH-07 Profile domain | ✅ | 范围合理 |
| ARCH-08 Protocol 单一事实源 | ✅ | 已完成，且进一步收敛 responsesMode |

## P1 — Draft metadata 被 Catalog 覆盖

当前 `BuildDraftPlan` 本身正确地保证“显式 Draft 值优先”。

但实际 HTTP Draft 路径为：

```text
draft
→ EnrichProposedModels
→ BuildDraftPlan
```

`EnrichProposedModels` 使用 `catalog.FillOverwrite`，会覆盖：

- contextWindow
- maxTokens
- input
- reasoning
- cost

因此用户显式编辑的 Draft metadata 可能在进入 `BuildDraftPlan` 前已被修改。

这与 `docs/system-contract.md §2.3` 的“显式 draft 优先”冲突。

### 建议

Generated 路径继续使用 overwrite enrich。

Draft 路径改为 missing-only enrich，例如使用 `catalog.FillMissing`，或提供一个很小的 Draft enrich helper。

必须补回归测试：

```text
catalog contextWindow = 1048576
draft contextWindow   = 111

最终 canonical draft 必须保持 111
```

同时覆盖：

- maxTokens
- reasoning
- input
- cost

## P2 — Gateway Current 仍有第二套读取逻辑

`gateway.ReadCurrent()` 已定义：

```text
文件不存在 → empty current
读取失败   → error
JSON 损坏  → error
```

但 `handleGetGateway()` 仍直接：

```go
os.ReadFile(...)
json.Unmarshal(...)
```

并把读取失败或损坏静默表现为 `gateway: null`。

### 建议

让 GET Gateway endpoint 也复用 Gateway 读取边界，不再由 server 自己定义 `models.json` 的错误语义。

## P2 — Generated Publish 仍构造第二份 Proposal

Generated publish 当前真正写盘走：

```text
BuildEnrichedGeneratedPlan
→ PublishPlan
```

但 HTTP handler 仍另外执行一次：

```go
BuildProposedGatewayEntry(cfg)
```

仅用于 auth caveat / success warning。

结果是：

```text
用于判断 warning 的 proposal
!=
真正发布的 canonical plan.Proposed
```

### 建议

让 warning 基于最终 `plan.Proposed`。

若第三方 provider 会误触发 warning，则让 `PublishedAuthCaveat` 只检查 pi-switch fixed providers，而不是保留第二份 proposal。

## P2/P3 — Profile Handler 仍有重复 raw validation

`handlePostProfile` 与 `handlePutProfile` 仍存在大段 raw JSON walker，用于提前检查 responsesMode。

而真实规则已经统一到：

```text
internal/protocol
→ protocol.ValidateResponsesMode
```

Profile domain 也已经调用同一规则。

### 建议

删除重复 raw walker，收敛为：

```text
HTTP JSON decode
→ typed ProviderProfile
→ profile domain validation
→ HTTP error mapping
```

Adapter 不再决定业务规则应该从哪些 JSON 路径执行。

## P3 — CreateProfile 未使用已有 ErrProfileExists

`internal/profile` 已建立 domain error taxonomy，包括：

```go
ErrProfileExists
```

但 `CreateProfile()` 对重名仍返回普通：

```go
errors.New("profile already exists")
```

### 建议

改为已有 typed/domain error，避免未来调用方重新依赖字符串匹配。

## 已确认落实良好的部分

### Config

当前已实现：

```text
ENOENT         → DefaultConfig
malformed JSON → error
read error     → error
field error    → error
```

并统一：

```text
ParseConfig
LoadConfigAtPath
SaveAtPath
```

写入流程已经收敛为：

```text
CreateTemp
→ 0600
→ Write
→ Sync
→ Close
→ Rename
```

没有引入 ConfigStore interface、file lock、revision 或 cache。

### TUI Stats

当前路径：

```text
TUI
→ stats.OpenSummaryService
→ stats.Summary
→ store
```

TUI 已不直接执行 SQL。

Summary 后续又复用了 `RequestFact.countable()`，与完整 Stats 共享 token / cost 统计语义。

### Profile

`internal/profile` 提取范围合理，仅迁移已有多入口消费者的能力，没有为了“完整领域层”强行搬空 server。

### Protocol

目前不仅统一了：

- API identifiers
- IsKnown
- CanProxy
- CanGateway

还进一步统一了：

- responsesMode allowed values
- default mode
- validation
- WebUI capability source

前端 fallback 使用受 Go parity test 约束的 fixture，而不是独立手写第二套规则，方向正确。

## 建议收尾顺序

按以下顺序处理：

1. **P1：修 Draft enrich，显式 Draft metadata 不得被 Catalog 覆盖。**
2. **P2：Gateway GET 复用统一读取边界。**
3. **P2：Generated publish warning 使用最终 canonical `plan.Proposed`。**
4. **P2/P3：删除 Profile handlers 的重复 raw responsesMode validation。**
5. **P3：CreateProfile 使用 `ErrProfileExists`。**

完成以上收尾后，再做一次小型静态复评即可。

## 停止建议

完成上述问题后，建议本轮架构改造正式停止。

暂不继续推进：

- Proxy core 大规模提取
- SQLite migration framework
- Typed HTTP DTO 全面改造
- TS 自动生成
- Gateway 全量类型化
- WebUI 大规模拆分

除非后续出现明确维护成本，否则不应仅因为“还能继续抽象”而继续重构。

## 最终评价

本轮落实的核心价值已经实现：

> 修改同一业务规则时，需要搜索和同步的地方明显减少。

尤其 Config、Gateway Generated flow、Stats、Protocol 四块已经形成较清晰的单一事实源。

当前最大的实质性缺口只有一个：

> `BuildDraftPlan` 本身正确，但 Draft 在进入它之前被 overwrite-enrich 修改。

修复该问题并完成上述少量收尾后，当前架构可认为达到本轮维护性改造目标。

## 收尾落实状态（同日）

| 项目 | 落实 |
|---|---|
| P1 Draft metadata 被 Catalog 覆盖 | `gateway.EnrichDraftModels`（`catalog.FillMissing`）成为 draft 路径唯一 enrich；Generated 仍 `FillOverwrite`。回归测试 `TestGatewayDraft_ExplicitMetadataBeatsCatalog` 覆盖 contextWindow / maxTokens / reasoning / input / cost，并在撤回修复时复现失败 |
| P2 Gateway Current 第二套读取逻辑 | `handleGetGateway` 复用 `gateway.ReadCurrent`：文件不存在为空 current，读取失败/JSON 损坏为 500，不再静默 `gateway: null` |
| P2 Generated Publish 第二份 Proposal | 两条 publish 路径的成功响应警告改用最终 `plan.Proposed`；`PublishedAuthCaveat` 只判定 pi-switch fixed provider，避免 current 中第三方 provider 误触发 |
| P2/P3 Profile raw validation | `handlePostProfile` / `handlePutProfile` 的 raw JSON walker 已删除，规则只从 typed `ProviderProfile` 判定 |
| P3 CreateProfile 域错误 | 重名返回 `profileErr(ErrProfileExists, ...)`，文案不变 |

验证：`scripts/test-limited.sh go`（`go test ./...`，`-p 1 -parallel 2`）全部通过。

据此，本报告列出的 1 个优先修复项与 4 个收尾项均已落实；本轮维护性改造到此停止，不再扩大重构范围。
