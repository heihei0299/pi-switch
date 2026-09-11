# pi-switch 最终复审与收尾执行计划

- 审查基准：`main @ 1faf98d`
- 审查范围：`8334e0b → 1faf98d`
- 审查方式：增量静态代码审查
- 未重新执行：build / test / lint / typecheck
- 目标：确认 FINAL-01～04 落实情况，并关闭剩余维护性问题

## 1. 最终复审结论

本轮 capability 收口整体完成质量很高。

已确认完成：

- `Known / CanProxy / CanGateway` capability contract 已统一。
- `PUT /api/config` 已拒绝 known-but-unproxyable API。
- `/api/config/validate` 能诊断磁盘上的不可代理 API。
- capability regression matrix 已建立。
- Preset 已过滤不可代理 API。
- WebUI 对已知但不可代理 API 保持可见但 disabled。
- Gateway / Config 继续复用同一 `Upstream.EffectiveAPI()`。
- `ProfileIssues()` 仍是 profile shape diagnostics 的唯一来源。
- 没有引入新的 service / repository / validator framework。

当前没有 P0 / P1 blocker。

剩余：

- **1 个 P2：Profile CRUD 的 flat profile 可绕过 capability 校验。**
- **2 个 P3：WebUI unknown API option 与 contract 文档清理。**

当前完成度约 **95%～97%**。

可维护性评价约：

```text
9.2 / 10
```

完成下面最后三个收尾项后，可认为达到约 **9.3 / 10**，并正式结束本轮 maintainability initiative。

---

## 2. 已完成项

### FINAL-01 — Known API / Proxy Capability

当前：

```go
ValidateEffectiveChannelAPI(...)
```

已经按以下顺序判断：

```text
API 缺失
→ API unknown
→ API known but CanProxy=false
→ responsesMode compatibility
```

known-but-unproxyable API 现在返回：

```text
api <id> is not currently proxy-supported
```

该规则直接来自：

```go
protocol.CanProxy(...)
```

没有新增第二份 supported API 列表。

### FINAL-02 — Advisory capability diagnostics

`GET /api/config/validate` 已能诊断旧配置、手工修改或降级遗留的不可代理 API。

legacy flat profile 会在：

```text
profiles.<name>.api
```

报告。

显式 channel 的 capability 问题通过：

```text
ProfileIssues
→ ValidateUpstreamAPI
→ ValidateEffectiveChannelAPI
```

报告到：

```text
profiles.<name>.upstreams[i].api
```

### FINAL-03 — Contract wording

当前 contract 已明确：

- loader 只保证配置能被解析和加载。
- `ResolvedUpstreams` 共享的是 effective upstream fallback 语义。
- 不保证 synthesized channel 一定能走完 route resolution。
- advisory 镜像的是 Profile 内容 validation，而不是 CRUD 的 rename/existence/map-key 等外层规则。

这些表述已经基本准确。

### FINAL-04 — Capability regression matrix

当前测试已显式锁定：

| API | Known | Proxy | Gateway | Config write |
|---|---:|---:|---:|---|
| openai-completions | yes | yes | yes | allow |
| openai-responses | yes | yes | yes | allow |
| anthropic-messages | yes | yes | no | allow |
| google-generative-ai | yes | no | no | reject |

新增 API 时必须决定 Config write policy，否则测试失败。

这个 guardrail 建议长期保留。

---

# 3. P2 — Profile CRUD flat profile 绕过 CanProxy

这是当前唯一值得阻塞“正式关闭”的问题。

严格 Profile 写入口：

```text
POST /api/profiles
PUT /api/profiles/:name
CLI provider add
        ↓
profile.ValidateProfile
```

当前 `ValidateProfile()`：

```go
ValidateResponsesMode
→ ValidateProviderProfile
→ ValidateProviderRetry
```

而 capability 校验目前主要在：

```go
ProfileIssues()
    ↓
for _, upstream := range p.Upstreams
    ↓
ValidateUpstreamAPI
    ↓
ValidateEffectiveChannelAPI
```

因此：

```text
len(p.Upstreams) == 0
```

时不会进入：

```go
ValidateEffectiveChannelAPI
```

### 可绕过案例

直接创建：

```json
{
  "name": "google",
  "profile": {
    "api": "google-generative-ai",
    "responsesMode": "auto",
    "baseUrl": "https://example.test/v1",
    "apiKey": "k"
  }
}
```

逻辑上：

```text
ValidateResponsesMode
→ OK

ProfileIssues
→ no upstreams
→ no CanProxy check

ValidateProviderRetry
→ OK

CreateProfile
→ persist
```

但：

```text
protocol.CanProxy("google-generative-ai") == false
```

所以 Profile CRUD 与 whole-file Config door 仍未完全一致。

未知 API 也存在同类风险，因为 `ValidateResponsesMode` 不负责 `IsKnown`。

### CLI 影响

`provider add` 在没有 `--base-url` 时构造：

```go
ProviderProfile{
    API: flags.api,
}
```

不会生成 upstream，因此也会走这个 flat-profile 缺口。

---

# 4. P2 修复计划

## CLOSE-01 — Flat Profile Capability Validation

### 修改目标

让所有 Profile CRUD 写入口都遵守：

```text
known + CanProxy=true  → allow
known + CanProxy=false → reject
unknown                → reject
```

### 推荐实现

只修改：

```go
profile.ValidateProfile(...)
```

不要新增 abstraction。

建议：

```go
func ValidateProfile(p config.ProviderProfile) error {
    if err := ValidateResponsesMode(p); err != nil {
        return err
    }

    if len(p.Upstreams) == 0 {
        if err := config.ValidateEffectiveChannelAPI(config.Upstream{}, p); err != nil {
            return err
        }
    }

    if err := ValidateProviderProfile(p); err != nil {
        return err
    }

    return config.ValidateProviderRetry(p)
}
```

语义：

```text
flat profile
→ profile API/mode 就是 effective pair

channel profile
→ 每个 channel 仍由 ProfileIssues / ValidateUpstreamAPI 判断
```

### 不做

不增加：

```text
ProfileCapabilityValidator
ValidationService
rule registry
validator interface
```

### 必需测试

至少增加：

```text
[ ] CreateProfile flat google-generative-ai → error
[ ] CreateProfile flat unknown-api → error
[ ] CreateProfile flat openai-responses → allow
[ ] POST /api/profiles flat google-generative-ai → 400
[ ] PUT /api/profiles/:name flat google-generative-ai → 400
[ ] CLI provider add --api google-generative-ai without --base-url → non-zero
```

其中核心 domain test 优先，HTTP/CLI 可按已有测试结构选择最小覆盖。

---

# 5. P3 — WebUI unknown API option 仍可选

当前已知但不可代理 API：

```text
google-generative-ai
```

会显示但 disabled。

但真正 unknown 的旧值走 fallback：

```tsx
{!protocolApiIds(caps).includes(apiType) && apiType && (
  <option value={apiType}>{apiType}</option>
)}
```

这个 option 没有 disabled。

因此 UI 当前表现：

```text
known but unsupported → disabled
unknown               → enabled
```

服务器保存仍会拒绝，因此不是数据完整性问题，但 UI contract 不完全一致。

## CLOSE-02 — Disable Unknown Current API

修改为：

```tsx
<option value={apiType} disabled>
  {apiType}
</option>
```

目标：

- 旧配置仍能显示自己的 unknown value。
- 不静默 fallback。
- 用户不能重新选择一个服务端必拒绝的值。

测试补充：

```text
preserves unknown api value without silent fallback
+
selected option disabled == true
```

---

# 6. P3 — system-contract 重复条目

当前 §2.2 末尾存在重复：

```text
7. 若要进一步收紧整文件门 ...
5. 若要进一步收紧整文件门 ...
```

内容重复。

## CLOSE-03 — Contract Cleanup

删除重复的第二条。

同时检查编号连续：

```text
1
2
3
4
5
6
7
```

不改语义。

---

# 7. 最终验证清单

完成 CLOSE-01～03 后，做最后一次小型复评。

检查：

```text
[ ] PUT /api/config 拒绝 CanProxy=false
[ ] Profile CRUD flat path 也拒绝 CanProxy=false
[ ] Profile CRUD flat unknown API 被拒绝
[ ] channel Profile 仍按 ValidateUpstreamAPI 校验
[ ] /api/config/validate 仍能诊断旧配置
[ ] Preset 仍只提供可代理 API
[ ] WebUI known unsupported API 显示但 disabled
[ ] WebUI unknown API 显示但 disabled
[ ] capability source 仍只有 internal/protocol
[ ] Gateway 仍复用 Upstream.EffectiveAPI
[ ] ProfileIssues 没有复制
[ ] 没有新增 service/interface/registry
[ ] system-contract 无重复条目
```

---

# 8. 建议执行顺序

```text
CLOSE-01
Flat Profile capability
      ↓
CLOSE-02
Unknown API UI disabled
      ↓
CLOSE-03
Contract cleanup
      ↓
最终静态复评
      ↓
STOP
```

---

# 9. Definition of Done

满足：

```text
[ ] Config door 与 Profile CRUD capability 一致
[ ] flat profile 不再绕过 IsKnown / CanProxy
[ ] UI 不提供服务端必拒绝的可选 API
[ ] advisory 继续能诊断历史坏配置
[ ] capability matrix 保持有效
[ ] contract 与实现一致
[ ] 没有新架构层
```

即可正式标记：

> **maintainability initiative CLOSED**

---

# 10. 明确停止继续重构

关闭后不主动推进：

```text
Proxy 大规模拆包
Repository / UseCase
DI framework
generic validation framework
Gateway 全量类型化
全面 HTTP DTO
全面 TS 自动生成
SQLite migration framework
WebUI 大规模重构
```

除非后续出现真实维护问题。

后续开发重点应转为：

> 控制新 API / protocol / provider / transport / retry policy 带来的组合复杂度，而不是继续整理架构。
