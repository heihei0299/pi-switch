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
[x] CreateProfile flat google-generative-ai → error
[x] CreateProfile flat unknown-api → error
[x] CreateProfile flat openai-responses → allow
[x] POST /api/profiles flat google-generative-ai → 400
[x] PUT /api/profiles/:name flat google-generative-ai → 400
[x] CLI provider add --api google-generative-ai without --base-url → non-zero
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
[x] PUT /api/config 拒绝 CanProxy=false
[x] Profile CRUD flat path 也拒绝 CanProxy=false
[x] Profile CRUD flat unknown API 被拒绝
[x] channel Profile 仍按 ValidateUpstreamAPI 校验
[x] /api/config/validate 仍能诊断旧配置
[x] Preset 仍只提供可代理 API
[x] WebUI known unsupported API 显示但 disabled
[x] WebUI unknown API 显示但 disabled
[x] capability source 仍只有 internal/protocol
[x] Gateway 仍复用 Upstream.EffectiveAPI
[x] ProfileIssues 没有复制
[x] 没有新增 service/interface/registry
[x] system-contract 无重复条目
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
[x] Config door 与 Profile CRUD capability 一致（两处已记录的有意差异，均不在能力维度：
    ①连 api 都没声明的 profile——整文件门与 duplicate 放行、CRUD 门与 advisory 拒收/报错；
    ②profile 顶层无 api 而 channel 自带 api——两个写门按 channel 的 effective api 放行、advisory 报 api required。
    见 system-contract §2.2 第 2 条）
[x] flat profile 不再绕过 IsKnown / CanProxy（含 `duplicate`：复制前按 `config.ValidateResolvedCapability` 判，与整文件门同一条；shape/retry 不判，见 §11）
[x] UI 不提供服务端必拒绝的可选 API
[x] advisory 继续能诊断历史坏配置
[x] capability matrix 保持有效
[x] contract 与实现一致
[x] 没有新架构层
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

---

# 11. 收尾执行记录（CLOSE-01～03 已完成）

验证：`scripts/test-limited.sh go webui`（`go test ./...` + vitest 全绿）。

| 项 | 状态 | 落点 | 回归证据 |
|---|---|---|---|
| CLOSE-01 Flat Profile capability | 完成 | `internal/profile/validate.go`：`ValidateProfile` 在 `len(p.Upstreams) == 0` 时走 `config.ValidateEffectiveFlatAPI(p)`（= `ValidateEffectiveChannelAPI(Upstream{}, p)` 的具名形式），有 channel 时保持原路径 | `TestCreateProfileFlatProfileObeysCapabilityContract`、`TestProfileCRUD_CapabilityMatrix`、`TestProfileCRUD_RejectsUnknownAPI`、CLI `TestHandleProvider_AddRefusesUnproxyableFlatAPI` |
| CLOSE-02 Unknown API UI | 完成 | `webui/src/components/ProfilesPanel.tsx`：unknown 当前值的 fallback option 加 `disabled`（仍可见、仍回显）；同类项一并对齐到 responsesMode 的 fallback option | `ProfilesPanel.test.tsx` "preserves unknown api value without silent fallback" 增加 `disabled === true`；`ProfilesPanel.responsesMode.test.tsx` "shows a legacy incompatible mode but keeps it unselectable" |
| CLOSE-03 Contract cleanup | 完成 | `docs/system-contract.md` §2.2：删掉重复条目（编号 1..7 连续）；Profile CRUD 行同步 flat profile 的 capability 判定 | — |

红→绿反证：把 CLOSE-01 的判定块临时置为不可达时，`TestProfileCRUD_CapabilityMatrix` 的 `POST /api/profiles` 与 `PUT /api/profiles/:name` 对 `google-generative-ai` 都返回 200（期望 400），domain 级 `CreateProfile` 返回 nil；恢复后全部转绿。CLOSE-02 的 `disabled` 断言同样先红后绿。

§7 最终验证清单全部通过：`PUT /api/config` capability matrix、Profile CRUD flat 路径、channel Profile 仍走 `ValidateUpstreamAPI`、advisory 仍能诊断旧配置、Preset 仍只提供可代理 API、WebUI 两种不可用值均 disabled、capability 单一来源仍为 `internal/protocol`、Gateway 仍复用 `Upstream.EffectiveAPI`、`ProfileIssues` 未复制、未新增 service/interface/registry、system-contract 无重复条目。

同类项（已一并处理）：`ProfilesPanel.tsx` 的 responsesMode fallback option 原先也没有 `disabled`，现按同一原则加上。一处诚实修正：它比 api 那个弱——`saveLocal` 先用 `responsesModeError`/`allowedResponsesModes` 拦下保存（客户端给出「passthrough requires openai-responses」这类话术），旧 mode 到不了服务端；而 api 那项此前没有任何客户端规则，unknown 值会一路走到 400。所以这一项是「不提供写入口必拒的可选项」的一致性对齐，不是补一个真实漏洞。

后续 code review（双轴：Standards + Spec；基准 `3a88157`）后的处理记录（三项全部执行；括号内为第二轮增量 review 的修正）：

- **① 整文件门补齐「无 channel 但声明了 api」的形状**：`ResolvedUpstreams()` 为空时按 profile 顶层 api 判一次。与 CRUD 门、advisory 门从此同一条规则。`config.Upstream{}` 这个读不出意图的哨兵收成具名调用 `config.ValidateEffectiveFlatAPI(profile)`：调用点是 CRUD 门的 flat 分支（`internal/profile/validate.go`）、advisory 门（`internal/server/profile_handlers.go`），以及 `ValidateResolvedCapability` 内部。判定的入口统一为 **`config.ValidateResolvedCapability`**（逐 channel + flat 那一格），整文件门与 duplicate 门共用它——共 3 处调用 `ValidateEffectiveFlatAPI` / 2 处调用 `ValidateResolvedCapability`，不是早先写的"三个调用点"。
- **② `duplicate` 门补能力判定**：`DuplicateProfile` 复制前调 `config.ValidateResolvedCapability`，即**副本不可能是整文件门会拒绝的配置**——flat 源与 channel 源一视同仁。早先只判 flat 是个说不通的中间态（同一个理由一半用一半不用，而且对逐字空的源比整文件门还严）。shape/模型/channel 名/retry 一律不判，磁盘上带这些问题的源仍能「复制一份再改」。
- **③ 两个 capability matrix 合并为一份共享期望** `capabilityWritePolicy` + `capabilityWriteCases`（`internal/server/config_diagnostics_test.go`）——两个门读同一份「新增 API 必须显式决策」表与同一条 CanProxy 一致性断言，各门仍保留自己的请求与断言逻辑。
- **不变量固化**：`internal/server/door_parity_test.go` 把「形状 × api → 三个门的判决」逐格声明成表——4 个形状（flat-no-url、flat-with-url、channel、channel-own-api）× 4 个 api（可代理、已知不可代理、未知、无 api）= **16 格**（早先写 16 格时表里只有 12 行，第二轮补上了 `flat-no-url × 无 api` 与 `channel-own-api` 三格）；duplicate 另有 10 行按「源形状」声明，含"逐字空的源与整文件门同样放行"。

反证：新表在实现前对 `flat-no-url` + `google-generative-ai` / `unknown-api` 两格、以及 duplicate 的 channel 不可代理 / channel 未知 api / 逐字空源三格为红（整文件门与 duplicate 都回 200，逐字空源那一格反倒是 duplicate 比门更严），实现后转绿；把 google 的写策略临时改成 `true`，两个 capability matrix 同时以 `CanProxy is the one source both write doors follow` 失败。

Final follow-up（基准 `main @ 42a25cd`，CLOSE-FINAL-01～06）：

- **duplicate 顶层 responsesMode parity 已修**：`PUT /api/config` 判两层（顶层 api/responsesMode 自洽 → `ValidateResolvedCapability`），而 duplicate 此前只判第二层，于是「顶层 `openai-completions + passthrough` 不可执行、channel 用 `openai-responses + passthrough` 覆盖」的 legacy profile 能被复制——那一格门会 400。现在 `DuplicateProfile` 按同一顺序判两层（顶层检查同样带 `prof.API != ""` 守卫，api 为空时与门一样跳过）。仍**不**调用完整 `ValidateProfile`：shape/模型/channel 名/retry 依旧不阻止复制。
- **targeted regression 已加**：domain 两条（`TestDuplicateProfileRejectsInvalidTopLevelResponsesModeEvenWhenChannelOverridesIt`、`TestDuplicateProfileAllowsValidTopLevelModeWithChannelOverride`）+ door parity 两格（`TestDoorParity_DuplicateMatchesWholeFileTopLevelModeRule`）。主矩阵刻意没有扩成 shape × api × responsesMode × channelMode。
- **stale legacy-flat 措辞已改**：`door_parity_test.go` 里那句"合成出的 channel 就是运行期那一个"改为声明 ResolvedUpstreams 的 fallback 语义、且只验证 capability 判定，不声明 synthesized channel 一定能走完 route resolution。
- **contract 已同步**：system-contract §2.2 的 duplicate 行改写为"顶层 mode 自洽 + `ValidateResolvedCapability` 两件事，副本不会是门因 mode/capability 而拒绝的配置"，并指回第 4 条。

红→绿：实现前 domain 的 A 例 `DuplicateProfile = nil`、door parity 的 A 格 `duplicate = 200`（期望 400），正例 A/B 均已通过（证明 channel override 本身不是错误）；实现后全绿。

> **maintainability initiative CLOSED — 无已知 P0 / P1 / P2。**
