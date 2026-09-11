# pi-switch 可维护性提升 Spec

- 基准：`main @ fd4c32b`
- 目标：在不重写、不换技术栈的前提下，减少重复规则、入口漂移和持久化旁路。

## 原则

1. 一个事实只有一个来源。
2. 一个业务动作只有一个实现。
3. Adapter 只负责输入、输出和展示，不复制业务规则。
4. 只抽取已经出现多个消费者的业务能力。
5. 不为“架构整洁”增加无实际需求的层和 interface。

## 本轮范围

### Config

`internal/config` 是配置读写唯一边界。

读取语义：

```text
文件不存在      → DefaultConfig
JSON 损坏       → error
权限 / I/O 错误 → error
字段类型错误    → error
```

写入要求：

```text
CreateTemp → 0600 → Write → Sync/Close → Rename
```

禁止 server、CLI、TUI 自行实现 config 写盘逻辑。

### Gateway

保留现有 Canonical Plan，但显式区分：

```text
Generated Plan = 根据 config 自动生成
Draft Plan     = 用户显式编辑
```

核心入口：

```go
BuildGeneratedPlan(...)
BuildDraftPlan(...)
PublishPlan(...)
```

禁止通过 `edited == nil` 推断业务语义。

WebUI、CLI、TUI 的 Generated preview/publish 必须走同一逻辑，并共享 metadata enrich、published metadata preservation、third-party preservation、validation 和 diff/conflict。

### Stats / TUI

统计语义统一归 `internal/stats`。

TUI 不直接访问 SQLite，不包含 `db.Query` / `db.QueryRow` 等 SQL 逻辑，只负责交互和展示。

### Profile

已经同时被 HTTP 与 CLI 使用的 Provider 业务，后续移出 `internal/server`，形成小型 `internal/profile` domain。

优先迁移：

- Create
- Duplicate
- FetchModels
- SetExposedModels
- TestUpstream

不要求一次迁完全部 profile handler。

### Protocol

统一 API 标识与支持能力，避免 `config`、`gateway`、`translator` 各维护一份列表。

建议提供：

```go
IsKnown(...)
CanProxy(...)
CanGateway(...)
```

保持简单，不建立复杂 capability framework。

## 不做

本轮不做：

- `internal/app` / UseCase / Repository 分层。
- 无第二实现的 interface。
- CQRS / Event Bus / DI Framework。
- Gateway 全量类型化。
- Proxy 全量重构。
- OpenAPI / TS 自动生成。
- Stats SQL 性能优化。
- Config cache。
- WebUI 大规模组件拆分。

## 完成标准

第一阶段完成后应满足：

- [ ] Config 错误不再被当成默认配置。
- [ ] `config.json` 新建权限为 0600。
- [ ] Config 保存只有一个实现。
- [ ] Gateway Generated / Draft 语义显式。
- [ ] CLI / TUI / WebUI Generated Gateway 行为一致。
- [ ] TUI 不直接访问 SQLite。

达到以上状态后先复评，再决定是否继续 Profile / Protocol 收敛。
