# pi-switch 最终改进计划

目标：

> 关闭当前维护性改造，只解决剩余 contract 一致性问题，不启动新的架构重构。

## FINAL-01 — 明确 Known API / Proxy Capability 策略

优先级：

```text
P2
```

### 目标

明确：

```text
IsKnown == true
CanProxy == false
```

时 Config 写入口应该如何处理。

代表案例：

```text
google-generative-ai
```

### 推荐决策

采用：

```text
整文件写入口只接受当前 CanProxy 的 effective API
```

即：

```text
known + proxy-supported → allow
known + not proxy-supported → reject
unknown → reject
```

### 修改位置

主要：

```text
internal/config/config.go
internal/server/profile_handlers.go
docs/system-contract.md
相关 tests
```

### 推荐实现

不要增加新抽象层。

可以直接让：

```go
ValidateEffectiveChannelAPI()
```

检查：

```go
if !protocol.IsKnown(api) {
    return fmt.Errorf("unsupported api %s", api)
}

if !protocol.CanProxy(api) {
    return fmt.Errorf("api %s is not currently proxy-supported", api)
}

return protocol.ValidateResponsesMode(...)
```

但需要确认这个函数是否还有不希望检查 `CanProxy` 的调用方。

若有，则拆成两个非常小的函数：

```go
ValidateEffectiveChannelCompatibility(...)
ValidateProxyableEffectiveChannel(...)
```

不要引入 validator interface。

### 测试

增加：

```text
google-generative-ai
IsKnown=true
CanProxy=false
```

case：

```text
PUT /api/config
→ 400
```

错误至少包含：

```text
google-generative-ai
proxy
```

同时锁：

```text
openai-completions → allow
openai-responses   → allow
anthropic-messages → allow
```

避免误伤已有能力。

## FINAL-02 — 对齐 advisory capability diagnostics

如果 FINAL-01 采用推荐方案：

`/api/config/validate` 对不可 Proxy API 应报告：

```text
error
```

即使通常这种配置不能再通过 PUT 写入，也需要支持：

```text
旧配置
手工修改磁盘
未来版本降级
```

场景。

建议输出：

```text
path:
profiles.<name>.upstreams[i].api

message:
api google-generative-ai is known but not currently proxy-supported
```

如果是 profile-level legacy flat profile，则路径可使用：

```text
profiles.<name>.api
```

### 原则

advisory 的能力诊断应来自：

```text
protocol.CanProxy
```

不能重新维护一份 supported-api 列表。

## FINAL-03 — 修正 Config contract 措辞

只改文档，不改结构。

### 修改 1

当前 legacy flat 描述中避免：

```text
ResolvedUpstreams 合成运行时同一个 channel
```

改为：

```text
ResolvedUpstreams 复用 runtime 的 effective upstream fallback 语义；
用于检查 legacy flat profile 的 effective API/mode。
```

### 修改 2

把：

```text
/api/config/validate 是完整 CRUD 门规则集镜像
```

收窄为：

```text
/api/config/validate 镜像 Profile 内容 validation，
并附加 config-level diagnostics。
```

这样不会暗示：

```text
profile map key/name
rename semantics
existence rules
```

也必须全部由 `ValidateProfile()` 覆盖。

## FINAL-04 — 补 capability regression matrix

建议增加一个很小的 table-driven test。

目标：

每一个：

```go
protocol.Capabilities()
```

返回的 API 都必须明确：

```text
IsKnown
CanProxy
CanGateway
AllowedResponsesModes
Config write expected result
```

例如：

| API | Known | Proxy | Gateway | Config write |
|---|---:|---:|---:|---|
| openai-completions | yes | yes | yes | allow |
| openai-responses | yes | yes | yes | allow |
| anthropic-messages | yes | yes | no | allow |
| google-generative-ai | yes | no | no | reject |

这样以后新增 API 时不会只改：

```text
IsKnown
```

忘记决定：

```text
CanProxy
CanGateway
Config behavior
```

## FINAL-05 — 做一次最终静态复评

完成 FINAL-01～04 后，只做最后一次小型 review。

检查：

```text
[ ] PUT /api/config 与 contract 一致
[ ] /api/config/validate 能发现旧的不可代理 API
[ ] protocol capability 没有第二份列表
[ ] Gateway effective API 仍复用 config helper
[ ] ProfileIssues 没有被复制
[ ] 没新增 interface / service / registry
```

如果全部通过：

> 正式关闭 maintainability initiative。

## 明确不做

这轮结束后，不继续：

```text
Proxy 大规模拆包
internal/app
UseCase 层
Repository 层
DI framework
Validation framework
Gateway typed model 重写
全面 HTTP DTO
全面 TS 自动生成
SQLite migration framework
WebUI 全面拆分
```

除非未来出现新的真实维护痛点。

## 后续开发原则

项目下一阶段重点不再是“整理架构”，而是：

> 控制新增功能带来的组合复杂度。

每次增加：

```text
新 API
新 protocol
新 provider
新 transport
新 responsesMode
新 retry policy
```

都必须明确回答：

```text
IsKnown?
CanProxy?
CanGateway?
AllowedResponsesModes?
Config 是否允许?
Gateway 是否发布?
WebUI 是否展示?
Runtime 是否可执行?
```

最好全部由已有 capability source 派生，而不是各层分别判断。

## 推荐执行顺序

```text
FINAL-01
Known/CanProxy contract
      ↓
FINAL-02
advisory capability diagnostic
      ↓
FINAL-03
contract wording
      ↓
FINAL-04
capability matrix test
      ↓
FINAL-05
最终静态复评
      ↓
STOP
```

预计这应该是一个非常小的收尾阶段。

如果执行过程中发现需要新增：

```text
framework
service layer
repository
generic validation engine
```

应直接停止，因为那已经超出本轮改造目标。

## Definition of Done

满足以下条件即可正式结束：

```text
[ ] Known-but-unproxyable API 行为明确
[ ] Config write 与 runtime capability 一致
[ ] Advisory 能报告不可代理 API
[ ] Protocol capability 是唯一事实源
[ ] Legacy flat contract 描述准确
[ ] Advisory/CRUD contract 措辞准确
[ ] Capability matrix 有 regression test
[ ] 无新增架构层
```

最终状态目标：

> pi-switch 的复杂度主要来自真实业务，而不是重复规则、隐藏 fallback 或多入口漂移。
