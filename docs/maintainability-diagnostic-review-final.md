# pi-switch 最新诊断 Review 报告

- 审查基准：`main @ 994a414`
- 审查范围：`a12f861 → 994a414`
- 审查方式：增量静态代码审查
- 未重新执行：build / test / lint / typecheck
- 重点：
  - Config tolerant write door
  - `/api/config/validate`
  - channel effective API / responsesMode
  - Profile validation
  - Protocol capability consistency

## 1. 总结

本轮改动质量总体较高。

上一轮留下的两个主要 P2 已基本关闭：

- `/api/config/validate` 已能够输出完整的 profile/channel/model shape diagnostics。
- `PUT /api/config` 已按 channel effective API / responsesMode 做兼容性判断。

同时还完成：

- 空 `responsesMode` 正确视为 `auto`。
- legacy/synthetic upstream 纳入 config-door 检查。
- profile/channel 均无 API 时拒绝保存。
- Gateway 删除自己的 effective API helper，统一复用 `config.Upstream.EffectiveAPI()`。
- `ProfileIssues()` 成为 shape diagnostics 的唯一来源。

当前没有 P0 / P1 blocker。

剩余问题主要是 **1 个 P2 contract/行为决策 + 2 个 P3 精度问题**。

## 2. P2 — Known API 与 Proxy Capability 不一致

当前协议层区分：

```text
IsKnown(api)
CanProxy(api)
CanGateway(api)
```

例如：

```text
google-generative-ai

IsKnown  = true
CanProxy = false
```

当前 `ValidateEffectiveChannelAPI()`：

```go
api := u.EffectiveAPI(profile.API)

if api == "" {
    return error
}

if !protocol.IsKnown(api) {
    return error
}

return protocol.ValidateResponsesMode(api, mode)
```

因此：

```json
{
  "api": "google-generative-ai",
  "responsesMode": "auto"
}
```

可以通过：

```text
PUT /api/config
```

但真实请求进入：

```text
translator.PlanRequest
→ upstreamFormat
→ protocol.CanProxy
```

后会失败：

```text
unsupported api
```

最终得到请求期错误。

### 2.1 当前 contract 存在自相矛盾

`system-contract.md` 目前同时表达了两个意思：

```text
整文件门拦截运行时 PlanRequest 必然拒绝的 effective api/mode
```

以及：

```text
IsKnown 但当前不可代理的 API 不在整文件门拦截
```

这两条不能同时完全成立。

因为：

```text
CanProxy == false
```

本身就意味着：

```text
PlanRequest 一定失败
```

### 2.2 必须明确策略

这里不建议继续靠局部条件修补。

应该先明确产品 contract。

有两个合法方案。

#### 方案 A — Config 写入口只接受当前可代理 API

规则：

```text
PUT /api/config
→ effective API 必须 CanProxy
```

即：

```go
if !protocol.CanProxy(api) {
    return fmt.Errorf("api %s is not currently proxy-supported", api)
}
```

优点：

- Config 写入口不会保存请求期必失败的 API。
- 与“阻止必然失败配置”的目标完全一致。
- 用户更早得到错误。
- contract 简单。

缺点：

- 不能提前保存未来准备支持、但当前未实现的 API。
- `google-generative-ai` 只能等 Proxy 实现后再使用。

#### 方案 B — 允许 Known 但不可 Proxy 的 API

规则：

```text
PUT /api/config
→ IsKnown 即可
```

但 contract 必须明确：

```text
整文件门只校验 API identity + mode compatibility，
不保证当前 Proxy 已实现该 API。
```

同时 `/api/config/validate` 必须输出：

```text
level: warning/error
path: profiles.<name>.upstreams[i].api
message: API is known but not currently proxy-supported
```

否则用户会得到：

```text
保存成功
validate 无问题
请求才失败
```

这是不可接受的诊断体验。

### 2.3 推荐

推荐 **方案 A**。

原因很简单：

`pi-switch` 当前 Config 的主要作用是驱动真实 Proxy/Gateway 行为，不是充当未来 provider schema storage。

如果某个 API 当前完全无法被请求路径执行：

```text
CanProxy == false
```

那么让配置写入口拒绝更符合：

```text
fail early
```

原则。

如果未来明确需要：

```text
“允许保存但暂不可运行”
```

再引入 capability warning 更合理。

## 3. P3 — legacy flat profile 的 runtime 描述不够准确

目前 Config door 使用：

```go
prof.ResolvedUpstreams()
```

来检查 legacy flat profile。

这本身没有问题。

问题在注释和 contract 对它的描述：

```text
ResolvedUpstreams 合成的 channel 就是 runtime 使用的同一个 channel
```

并不完全准确。

当前 route resolution 的第一阶段仍然主要遍历：

```go
prof.Upstreams
```

例如：

```go
resolveRoute()
```

依赖：

```text
channel.ExposedModels
```

寻找模型。

所以一个真正：

```text
prof.Upstreams == nil
```

的旧 flat profile：

虽然：

```go
ResolvedUpstreams()
```

可以生成 baseURL/APIKey fallback upstream，

但不代表它一定可以完整经过：

```text
model discovery
→ route resolution
→ channel pinning
```

的运行时路径。

### 建议

代码无需因此修改。

只修文档/注释：

不要写：

```text
ResolvedUpstreams 合成运行期同一个 channel
```

改成：

```text
Config door 使用与 runtime 相同的 effective upstream fallback 语义，
因此 legacy flat profile 的 API/baseURL/APIKey fallback 也参与兼容性检查。
```

更准确。

## 4. P3 — `/api/config/validate` 并非严格意义上的完整 CRUD 镜像

当前：

```go
ProfileIssues()
```

很好地统一了：

- baseURL
- channel name
- duplicate channel
- channel API
- model ID
- duplicate model
- exposedModels

`handleValidate()` 另外补：

- profile API
- responsesMode
- retry
- settings retry
- modelsDevProvider
- no models

整体已经相当完整。

但 Profile CRUD 仍有一个 config-map 外层规则：

```go
CreateProfile(name, ...)
```

会检查：

```text
profile name 非空
```

而：

```json
{
  "profiles": {
    "": {
      ...
    }
  }
}
```

这种 config key 并不属于：

```go
ValidateProfile(profile)
```

所以 advisory 并不是真正 100% 镜像所有 CRUD 行为。

### 建议

有两个简单选择。

#### 选择 1

修改 contract 措辞：

```text
/api/config/validate 镜像 Profile 内容 validation
```

而不是：

```text
镜像完整 CRUD 门规则集
```

这是最小方案。

#### 选择 2

在 config-level validator 加：

```text
profiles.<name>
```

key validation，例如：

```go
if strings.TrimSpace(name) == "" {
    issue(...)
}
```

如果 Profile name 还有字符集/长度规则，也应在这里统一。

当前只有“非空”规则时，更倾向选择 1，不值得为一句“完全镜像”再加体系。

## 5. ProfileIssues 评价

`ProfileIssues()` 是本轮比较成功的改动。

设计：

```text
ProfileIssues
    ↓
返回全部 Issue

ValidateProviderProfile
    ↓
返回第一条 Issue
```

让两个场景自然复用：

```text
严格写入口
→ 第一个错误

advisory validate
→ 所有错误
```

没有引入：

```text
Validator interface
Rule registry
Validation pipeline framework
```

抽象程度合适。

这块建议保持现状。

## 6. Effective API 收敛评价

Gateway 之前自己的：

```go
effectiveChannelAPI()
```

已经删除。

现在统一：

```go
Upstream.EffectiveAPI()
```

并被：

```text
Config
Gateway
```

复用。

这是正确方向。

长期应该保持：

```text
effective API
effective responsesMode
```

都只有一个 owner。

不要再在 Server/Gateway/Translator 新增本地 fallback 逻辑。

## 7. 当前状态

| 项目 | 状态 |
|---|---|
| Config strict load/write | ✅ |
| Generated/Draft Gateway | ✅ |
| Draft metadata preservation | ✅ |
| Gateway read boundary | ✅ |
| TUI Stats 去 SQL | ✅ |
| Profile domain | ✅ |
| Protocol single source | ✅ |
| Full profile diagnostics | ✅ |
| Channel effective mode validation | ✅ |
| Empty mode = auto | ✅ |
| no API anywhere rejection | ✅ |
| Effective API helper unification | ✅ |
| Known-but-unproxyable API policy | ⚠️ P2 |
| Legacy flat runtime wording | ⚠️ P3 |
| “complete CRUD mirror” wording | ⚠️ P3 |

## 8. 最终判断

当前项目已经没有值得继续做大型结构调整的问题。

剩余核心问题不是架构，而是：

> Known API 与 Runtime Capability 的产品 contract 需要最后确定。

完成这一点后，这轮 maintainability 工作应正式结束。

当前维护性评价：

```text
9.2 / 10
```

若 capability contract 收口：

```text
约 9.3 / 10
```

继续追求更高分数大概率需要付出不值得的抽象成本。
