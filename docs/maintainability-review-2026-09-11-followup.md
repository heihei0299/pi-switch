# pi-switch 可维护性后续审查报告

- 审查基准：`702c530 → 8dd20ade`
- 日期：2026-09-11
- 审查方式：增量静态代码审查
- 未重新执行：build / test / lint / typecheck
- 对照文档：
  - `docs/maintainability-spec.md`
  - `docs/maintainability-plan.md`
  - `docs/maintainability-review-2026-09-11.md`
  - `docs/system-contract.md`

## 结论

上一轮审查的 1 个 P1 + 4 个收尾项已经基本完成。

本轮未发现新的 P0 / P1 blocker。

当前仍建议处理：

1. **P2：Draft `name` 的 enrich 语义与 contract 不一致**
2. **P3：`PUT /api/config` 仍绕过统一的 Profile validation**
3. **Test gap：duplicate profile 未锁定 `ErrProfileExists` error kind**

完成以上小项后，建议正式停止本轮架构重构。

当前可维护性评价：约 **9.2/10**。

---

## 已关闭：Draft metadata overwrite

上一轮 P1：

```text
Draft
→ EnrichProposedModels
→ FillOverwrite
→ BuildDraftPlan
```

已改为：

```text
Generated → FillOverwrite
Draft     → FillMissing
```

新的 Draft 路径已经覆盖：

- contextWindow
- maxTokens
- reasoning
- input
- cost

并确认 Generated 路径仍会使用 catalog 刷新陈旧 metadata。

该问题主体已关闭。

---

## P2 — Draft `name` 仍违反 contract

`docs/system-contract.md §2.3` 当前规定：

```text
name / reasoning / cost 子字段按“键是否存在”判断是否已声明。
```

但 `catalog.FillMissing` 对 name 的实现仍是：

```go
if s, _ := entry["name"].(string);
    strings.TrimSpace(s) == "" && strings.TrimSpace(meta.Name) != "" {
    entry["name"] = meta.Name
}
```

因此：

```json
{"name": ""}
```

虽然键明确存在，仍会被 catalog name 覆盖。

前端 `modelPreview()` 可以保留显式空 name，因此真实路径存在：

```text
用户明确清空 name
→ Draft enrich
→ catalog name 被重新写回
```

这与“显式 Draft 优先”不一致。

### 建议

按键存在判断：

```go
if _, has := entry["name"]; !has && strings.TrimSpace(meta.Name) != "" {
    entry["name"] = meta.Name
    filled = true
}
```

增加回归测试：

```text
draft name = ""
catalog name = "Flash X"

published name 必须保持 ""
```

若非字符串 name 属于非法输入，应由 validation 拒绝，而不是由 enrich 静默纠正。

---

## P3 — `PUT /api/config` 仍绕过统一 Profile validation

最新代码增加：

```go
profile.ValidateProfile(...)
```

并统一了：

```text
POST /api/profiles
PUT /api/profiles/:name
```

的校验顺序：

```text
responsesMode
→ profile shape
→ retry
```

但：

```text
PUT /api/config
```

目前仍只执行：

```go
protocol.ValidateResponsesMode(...)
```

没有复用：

```go
profile.ValidateProfile(...)
```

因此完整 Config 写入路径仍可能接受 Profile CRUD 路径会拒绝的内容，例如：

- 非 http/https upstream baseUrl
- 重复 channel name
- exposedModels 不属于 model pool
- 超出允许范围的 requestRetry

### 建议

`handlePutConfig` 对每个 profile 调用：

```go
profile.ValidateProfile(prof)
```

保持 Profile business validation 单一来源。

若 `PUT /api/config` 有意允许保存运行时不支持的 profile，则必须在 contract 中显式说明这个差异。

---

## Test gap — duplicate profile 未锁定 error kind

`CreateProfile` 已修为：

```go
profileErr(ErrProfileExists, "profile already exists")
```

实现正确。

但当前测试只检查：

```go
err != nil
```

建议补：

```go
if !errors.Is(err, ErrProfileExists) {
    ...
}
```

防止未来重新退化为字符串错误。

---

## 已确认修复良好的部分

### Gateway Read Boundary

`handleGetGateway` 已统一走：

```text
gateway.ReadCurrent
```

语义：

```text
文件不存在 → empty current
读取失败   → 500
JSON 损坏  → 500
正常       → current projection
```

server 不再自行读取并吞错。

### Generated Publish 单一路径

publish warning 已改为直接判断最终：

```go
plan.Proposed
```

不再额外调用 `BuildProposedGatewayEntry` 构造第二份 proposal。

warning 与实际发布内容使用同一 canonical result。

### Auth caveat

当前只在以下条件成立时警告：

```text
存在 pi-switch fixed provider
+
provider 至少包含一个可路由 model
+
proxy 暴露到 loopback 之外或 fixed provider baseUrl 指向 LAN
```

不会因为：

- 仅存在第三方 provider
- fixed provider models=[]

产生无意义 warning。

`apiKey` 内容不参与判断也是合理的：fixed provider 被客户端作为 Bearer 调用，而暴露的 proxy 需要 Basic。

### Draft cost=0

cost 子字段现在按键是否存在判断。

显式：

```json
{"input": 0}
```

表示已知零价，不会被 catalog 覆盖。

缺失字段仍可由 catalog 补齐。

该语义和 Stats 中“0 != unknown”的规则一致。

### Profile raw validation

Profile handlers 中原有的 raw JSON responsesMode walker 已删除。

现在：

```text
HTTP decode
→ typed ProviderProfile
→ profile validation
→ HTTP error mapping
```

Adapter 不再复制 responsesMode 业务判断。

### ValidateProfile

新增 `ValidateProfile` 只是对现有三个 validation 的组合：

```go
ValidateResponsesMode
ValidateProviderProfile
ValidateProviderRetry
```

它被 Create 与 PUT profile 两条路径共享，没有引入 Service/interface 等额外层，抽象程度合适。

### Script executable bit

最新提交恢复 `scripts/*.sh` 的 executable bit，与 README / AGENTS 的直接执行方式一致。

仅 mode change，没有代码语义问题。

---

## 建议处理顺序

1. 修 `FillMissing` 的显式空 `name` 语义。
2. 让 `PUT /api/config` 复用 `profile.ValidateProfile`。
3. 补 `ErrProfileExists` error-kind test。
4. 做一次小型静态复评后停止架构改造。

---

## 停止建议

以上问题处理完成后，不建议继续主动推进：

- Proxy core 大规模提取
- Repository / UseCase 层
- SQLite migration framework
- 全面 Typed DTO
- Gateway 全量类型化
- WebUI 大规模拆分

除非出现明确维护成本，否则继续抽象的收益已经开始明显递减。

当前项目的主要维护目标应从“继续整理架构”转为：

> 控制新增功能带来的组合复杂度，防止 protocol / provider / transport / retry 等维度继续互相相乘。
