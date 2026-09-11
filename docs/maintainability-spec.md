# pi-switch 可维护性提升 Spec

- 基准：`main @ fd4c32b`
- 目标：不重写、不换技术栈，消除重复规则和跨入口行为漂移。

## 1. 核心原则

> 一个事实一个来源，一个业务动作一个实现。

本轮重点：

1. Config 读写语义统一。
2. Gateway CLI / TUI / WebUI 行为统一。
3. TUI 不再维护 Stats SQL。
4. HTTP 与 CLI 共享的 Profile 业务逐步移出 `server`。
5. Protocol 支持范围只有一个事实来源。

## 2. 不做

本轮不引入：

- `internal/app` / UseCase / Repository 层。
- 无第二实现的 interface。
- CQRS / Event Bus / DI Framework。
- Gateway 全量类型化。
- 全面 API 重写。

保留 Gin、Bubble Tea、SQLite、React。

## 3. Config

只有配置文件不存在时允许返回默认配置：

```text
不存在       → DefaultConfig
JSON 损坏    → error
权限/I/O错误 → error
字段类型错误 → error
```

所有配置写入统一由 `internal/config` 完成。

保存要求：

```text
CreateTemp
→ 0600
→ Write
→ Close/Sync
→ Rename
```

禁止：

- server / TUI / CLI 自己写 `config.json`。
- 固定 `config.json.tmp`。
- 吞掉保存错误。

## 4. Gateway

保留现有 Canonical Plan。

显式区分：

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

WebUI、CLI、TUI 的 Generated 行为必须一致，并统一包含：

- proposal
- metadata enrich
- published metadata preservation
- third-party provider preservation
- validation
- diff/conflict

连续发布相同配置时应满足：

```text
PendingCount == 0
```

## 5. Stats / TUI

统计语义统一归 `internal/stats`。

TUI 不得直接：

- `GetDB`
- `db.Query`
- `db.QueryRow`

TUI 只负责交互、展示和格式化。

## 6. Profile

已经同时被 HTTP 和 CLI 使用的 Provider 业务，逐步移出 `internal/server`。

目标：

```text
HTTP ─┐
      ├→ internal/profile
CLI ──┘
```

优先迁移：

- Create
- Duplicate
- FetchModels
- SetExposedModels
- TestUpstream

不要求一次搬完整个 `profile_handlers.go`。

## 7. Protocol

统一 API 标识和能力判断。

建议 `internal/protocol` 提供：

```go
IsKnown(...)
CanProxy(...)
CanGateway(...)
```

避免 `config`、`gateway`、`translator` 各维护一份支持列表。

`translator` 继续只负责协议转换。

## 8. 后续项

以下不进入第一阶段：

- Proxy core 提取。
- SQLite versioned migration。
- Typed HTTP DTO。
- TS 类型自动生成。
- Gateway typed model。
- Stats SQL 优化。
- WebUI 大组件拆分。

只有出现明确维护成本时再做。

## 9. 架构约束

长期保持：

- TUI 不写 SQL。
- Domain package 不依赖 server。
- Adapter 不复制业务规则。
- Config 写入只有一个实现。
- Gateway Generated flow 只有一个实现。
- Protocol capability 只有一个来源。

## 10. 第一阶段完成标准

- [ ] Config 错误不再被当成默认配置。
- [ ] `config.json` 新建权限为 0600。
- [ ] Config 保存只有一个实现。
- [ ] Gateway Generated / Draft 语义显式。
- [ ] CLI / TUI / WebUI Generated Gateway 一致。
- [ ] TUI 不直接访问 SQLite。

达到这些条件后暂停并重新审查，不继续机械重构。
