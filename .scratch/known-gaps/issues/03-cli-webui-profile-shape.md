# 03: 「CLI 与 WebUI 建出的 profile 形状分叉」——核验后不成立

Status: **不成立（撤回）** —— 记录在此以免被重新提出；无需修复

## 原判断（来自双轴 review 的 Spec 轴，我此前在对话中也重复过）

> `provider add` 新增了合成渠道 `main` 与 `--models`，使 CLI 建的 profile 与 WebUI 建的分叉：
> `ProfilesPanel.tsx:479` 仍提交顶层 `baseUrl` 旧形状，而两者号称共用同一个 handler。

## 反证（已核到行号）

1. **WebUI 也会合成 `main` 渠道**：`webui/src/components/ProfilesPanel.tsx:338-359` 的 `build()`：
   `if (!next.upstreams || next.upstreams.length === 0)` → 用顶层字段合成
   `upstreams = [{name: "main", api, responsesMode: next.responsesMode ?? "auto", baseUrl, apiKey, headers,
   models: next.models ?? [], exposedModels: next.exposedModels ?? []}]`，并 `delete next.models /
   next.exposedModels`。这与 CLI 的 `provider add`（`cmd/pi-switch/main.go` 的 add 分支：顶层
   `api/baseUrl/apiKey` + 给了 `--base-url` 时建 `upstreams[0] = {Name:"main", ...}`）**形状一致**：
   两边都是「顶层 baseUrl/apiKey + 一个名为 main 的渠道，携带同样的值」。
2. **被引用的 `:479` 不是提交载荷**：那是 `upstreams.length === 0` 时显示的 "Manage upstreams" 按钮，
   点击后把顶层字段**预填**进一个新 upstream（UI 播种动作）。它不决定保存时的形状——保存由 `build()`
   决定，而 `build()` 见第 1 条。
3. **唯一的字段差异是无行为影响的**：WebUI 的渠道带 `responsesMode: "auto"` 与 `headers`，CLI 渠道不带；
   而空 `responsesMode` 在 5 处都被规范化为 `"auto"`：`internal/translator/translator.go:18`、
   `internal/config/config.go:68`、`internal/translator/registry.go:157`、
   `internal/server/profile_handlers.go:93` 与 `:1001`。WebUI 另设 `updatedAt`，CLI 不设。

## 核验中确实发现的一处小差异（不构成缺陷，仅记录）

**不给 `--base-url` 时**：CLI 建的 profile **0 个渠道**；WebUI 建的 profile **1 个渠道且 baseUrl 为空**。

后果都不静默：渠道数为 0 时 `provider expose` 现在报
`profile "x" has no channels; add one with a baseUrl before exposing`（上一批次修复）；
WebUI 那条路会在后续 expose 时以 `unknown model` 失败。**没有**任何一条会假装成功，故不需要为它开票。

## 结论与教训

原判断来自「把 UI 播种按钮当成提交载荷」这一处读码偏差，属**未经执行验证的推断**。
本文件中三条目里唯一被推翻的一条，也正说明为什么每条目都要把证据写到文件:行——
写成"WebUI 仍提交旧形状"这种结论时，它看上去和另外两条一样可信。
