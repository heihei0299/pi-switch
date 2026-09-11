# pi-switch 可维护性提升执行计划

执行规则：

- 一个 Ticket 一个目标。
- 一个 Ticket 一个 PR。
- 不顺手扩大范围。
- 先修行为边界，再改内部结构。
- build / test / lint / typecheck 需明确授权后执行。

## 执行记录

基线 `6a474d1`；ARCH-01～08 与第一检查点均已完成，另有两项两轴 review 驱动的收敛。
每个 ticket 一个 commit：

| Ticket | commit | 内容 |
|---|---|---|
| ARCH-01 | `2271144` | fix(config): make config loading fail explicitly |
| harness | `6260f58` | build(scripts): 限制 Go 编译/测试并发 |
| ARCH-02 | `c1d93e1` | fix(config): centralize secure atomic persistence |
| ARCH-03 | `596bc47` | refactor(gateway): make generated and draft planning explicit |
| ARCH-04 | `73e9f9f` | refactor(gateway): unify generated gateway flow |
| ARCH-05 | `3e9a181` | refactor(gateway): share generated metadata enrichment |
| ARCH-06 | `35c717b` | refactor(tui): reuse stats service |
| 第一检查点清理 | `4f59d98` | refactor(gateway): tighten generated plan surface |
| 两轴 review 修复 | `a9a4aa4` | fix(config,stats): address the two-axis review findings |
| 计划外（信封） | `7655853` | refactor(api): one error envelope per surface |
| 计划外（信封） | `d1759e4` | fix(api,config): close envelope gap and de-shadow the null rule |
| 计划外（信封） | `202b236` | fix(api,config): enforce one envelope per surface end to end |
| ARCH-07 | `4445f34` | refactor(profile): extract shared profile domain |
| ARCH-08 | `2bfd5aa` | refactor(protocol): centralize protocol capabilities |
| 计划外（responsesMode） | `891d0da` | refactor(protocol): single responsesMode rule, exposed to the WebUI |
| 计划外（responsesMode） | `64ebbc1` | fix(webui,protocol): capability-driven responsesMode UI, pinned fallback |
| 发版 | `d14e354` | chore(release): bump version to 20260911.0.0 |

计划外两项都来自 review：HTTP 错误信封契约（`docs/system-contract.md` §2.8）与
responsesMode/API 能力单一来源。ARCH-07 顺带把 retry 校验下沉到 `internal/config`，
使 server 写路径与运行时分类共用一份。发版版本 `20260911.0.0`。

## ARCH-01 — Config 严格读取

- [x] 只有 `os.ErrNotExist` 返回 `DefaultConfig`
- [x] malformed JSON 返回 error
- [x] permission / I/O error 返回 error
- [x] 关键字段 Unmarshal 不再吞错
- [x] 删除为宽松 loader 写的重复校验

建议 commit：

```text
fix(config): make config loading fail explicitly
```

## ARCH-02 — Config 统一安全写入

- [x] 增加统一 atomic writer
- [x] 使用 `CreateTemp`
- [x] temp / config 文件权限为 0600
- [x] `SaveAtPath` 统一走 writer
- [x] `handlePutConfig` 不直接 `WriteFile/Rename`
- [x] raw JSON 先解析为 typed config 再保存
- [x] 删除固定 `.tmp` 文件名

不做 ConfigStore、file lock、revision、cache。

建议 commit：

```text
fix(config): centralize secure atomic persistence
```

## ARCH-03 — Gateway 显式区分 Generated / Draft

- [x] 增加 `BuildGeneratedPlan`
- [x] 增加 `BuildDraftPlan`
- [x] 保留 `PublishPlan`
- [x] 业务语义不再依赖 nil 判断

建议 commit：

```text
refactor(gateway): make generated and draft planning explicit
```

## ARCH-04 — Gateway 三入口统一

- [x] WebUI Generated preview/publish → `BuildGeneratedPlan`
- [x] CLI preview/publish → `BuildGeneratedPlan`
- [x] TUI preview/publish → `BuildGeneratedPlan`
- [x] WebUI 手工编辑 → `BuildDraftPlan`
- [x] 删除入口自行拼 Generated flow 的旁路

建议 commit：

```text
refactor(gateway): unify generated gateway flow
```

## ARCH-05 — Gateway Enrich 统一

- [x] 把 Generated Gateway 所需 enrich 提成共享实现
- [x] WebUI / CLI / TUI 使用同一 enrich
- [x] 保留 published metadata preservation
- [x] 保留 third-party provider preservation
- [x] 不增加 GatewayEnricher interface / Repository abstraction

建议 commit：

```text
refactor(gateway): share generated metadata enrichment
```

## ARCH-06 — TUI Stats 去 SQL

- [x] `stats.Service` 增加 Summary
- [x] TUI 删除 `internal/store` import
- [x] TUI 删除 SQL
- [x] TUI 改调用 `stats.Service`
- [x] TUI 只保留展示格式化

建议 commit：

```text
refactor(tui): reuse stats service
```

## 第一检查点

ARCH-01～06 完成后暂停复评：

- [x] Config 是否只剩一个可信读写边界
- [x] Gateway 是否只剩一个 Generated flow
- [x] CLI / TUI / WebUI 是否行为一致
- [x] TUI 是否已没有数据层旁路
- [x] 是否出现新的无必要抽象

如果以上状态良好，再考虑第二阶段。

## ARCH-07 — 提取共享 Profile 业务

仅迁已经被 HTTP + CLI 同时使用的逻辑：

- [x] Create
- [x] Duplicate
- [x] FetchModels
- [x] SetExposedModels
- [x] TestUpstream
- [x] 相关 errors / validation

目标：

```text
HTTP ─┐
      ├→ internal/profile
CLI ──┘
```

不加 interface。

建议 commit：

```text
refactor(profile): extract shared profile domain
```

## ARCH-08 — Protocol 单一事实源

- [x] 新增 protocol API constants
- [x] `IsKnown`
- [x] `CanProxy`
- [x] `CanGateway`
- [x] config 删除独立 allowed list
- [x] gateway 删除独立 support list
- [x] translator 使用统一 API identity

保持简单 `switch`，不做复杂 registry。

建议 commit：

```text
refactor(protocol): centralize protocol capabilities
```

## 停止条件

完成 ARCH-01～06 后必须先复评；ARCH-07～08 只有确认仍有明确收益时再执行。

以下事项不作为当前必做项：

- Proxy core 提取
- SQLite migration framework
- Typed HTTP DTO
- TS 类型生成
- Gateway typed model
- Stats SQL 优化
- WebUI 大组件拆分

不要因为“计划里写了”就机械执行。
