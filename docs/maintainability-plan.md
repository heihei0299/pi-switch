# pi-switch 可维护性提升执行计划

执行规则：

- 一个 Ticket 一个目标。
- 一个 Ticket 一个 PR。
- 不顺手扩大范围。
- 先修行为边界，再改内部结构。
- build / test / lint / typecheck 需明确授权后执行。

## ARCH-01 — Config 严格读取

- [ ] 只有 `os.ErrNotExist` 返回 `DefaultConfig`
- [ ] malformed JSON 返回 error
- [ ] permission / I/O error 返回 error
- [ ] 关键字段 Unmarshal 不再吞错
- [ ] 删除为宽松 loader 写的重复校验

建议 commit：

```text
fix(config): make config loading fail explicitly
```

## ARCH-02 — Config 统一安全写入

- [ ] 增加统一 atomic writer
- [ ] 使用 `CreateTemp`
- [ ] temp / config 文件权限为 0600
- [ ] `SaveAtPath` 统一走 writer
- [ ] `handlePutConfig` 不直接 `WriteFile/Rename`
- [ ] raw JSON 先解析为 typed config 再保存
- [ ] 删除固定 `.tmp` 文件名

不做 ConfigStore、file lock、revision、cache。

建议 commit：

```text
fix(config): centralize secure atomic persistence
```

## ARCH-03 — Gateway 显式区分 Generated / Draft

- [ ] 增加 `BuildGeneratedPlan`
- [ ] 增加 `BuildDraftPlan`
- [ ] 保留 `PublishPlan`
- [ ] 业务语义不再依赖 nil 判断

建议 commit：

```text
refactor(gateway): make generated and draft planning explicit
```

## ARCH-04 — Gateway 三入口统一

- [ ] WebUI Generated preview/publish → `BuildGeneratedPlan`
- [ ] CLI preview/publish → `BuildGeneratedPlan`
- [ ] TUI preview/publish → `BuildGeneratedPlan`
- [ ] WebUI 手工编辑 → `BuildDraftPlan`
- [ ] 删除入口自行拼 Generated flow 的旁路

建议 commit：

```text
refactor(gateway): unify generated gateway flow
```

## ARCH-05 — Gateway Enrich 统一

- [ ] 把 Generated Gateway 所需 enrich 提成共享实现
- [ ] WebUI / CLI / TUI 使用同一 enrich
- [ ] 保留 published metadata preservation
- [ ] 保留 third-party provider preservation
- [ ] 不增加 GatewayEnricher interface / Repository abstraction

建议 commit：

```text
refactor(gateway): share generated metadata enrichment
```

## ARCH-06 — TUI Stats 去 SQL

- [ ] `stats.Service` 增加 Summary
- [ ] TUI 删除 `internal/store` import
- [ ] TUI 删除 SQL
- [ ] TUI 改调用 `stats.Service`
- [ ] TUI 只保留展示格式化

建议 commit：

```text
refactor(tui): reuse stats service
```

## 第一检查点

ARCH-01～06 完成后暂停复评：

- [ ] Config 是否只剩一个可信读写边界
- [ ] Gateway 是否只剩一个 Generated flow
- [ ] CLI / TUI / WebUI 是否行为一致
- [ ] TUI 是否已没有数据层旁路
- [ ] 是否出现新的无必要抽象

如果以上状态良好，再考虑第二阶段。

## ARCH-07 — 提取共享 Profile 业务

仅迁已经被 HTTP + CLI 同时使用的逻辑：

- [ ] Create
- [ ] Duplicate
- [ ] FetchModels
- [ ] SetExposedModels
- [ ] TestUpstream
- [ ] 相关 errors / validation

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

- [ ] 新增 protocol API constants
- [ ] `IsKnown`
- [ ] `CanProxy`
- [ ] `CanGateway`
- [ ] config 删除独立 allowed list
- [ ] gateway 删除独立 support list
- [ ] translator 使用统一 API identity

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
