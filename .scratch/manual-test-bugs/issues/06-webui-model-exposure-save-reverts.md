# 06 — WebUI 模型暴露保存失败，网关候选列表回退到旧模型

**现象：** 在供应商的「模型」窗口勾选一个原来未暴露的模型，点击「保存」后，页面刷新仍只显示旧的暴露模型；进入「网关」页面，候选分组也继续使用旧列表。

**复现步骤：**

1. 在隔离的 `pi-switch-web` 容器中启动新 Go 二进制 `20260908.0.2` 的真实嵌入式 WebUI。
2. 准备一个 `mock/main` channel：模型为 `model-a`、`model-b`，初始 `exposedModels=["model-a"]`。
3. 用真实 Chromium 打开容器 WebUI，进入「供应商 → 模型」。
4. 勾选 `model-b`，确认 checkbox 从 `false` 变为 `true`，点击「保存」。
5. 检查浏览器请求和隔离 config。

**预期：**

- `PUT /api/profiles/mock/models` 成功后，必须继续发送 `PUT /api/profiles/mock/expose?channel=main`。
- `exposedModels` 应变成 `["model-a","model-b"]`。
- 刷新供应商和网关页面后，`model-b` 应出现在暴露模型及网关候选分组中。

**实际：**

- `PUT /api/profiles/mock/models` 返回 `200`。
- 浏览器没有发送 `PUT /api/profiles/mock/expose?channel=main`。
- 隔离 config 仍为 `"exposedModels":["model-a"]`。
- 网关页面因此继续显示旧的 `model-a`；“网关保存后仍显示以前的模型”是该失败向下游的表现。
- 直接在网关页面选择一个已经暴露的模型并执行「应用到 Pi」能够成功，说明本轮网关发布链路本身不是首要根因。

**根因：** 后端 models 更新响应固定包含 `"backup":null`：

```json
{"ok":true,"backup":null,"enrich":{"enriched":0}}
```

`webui/src/apiSchema.ts` 的 `decodeUpdateModels` 使用 `optionalString(raw, "backup", ...)`，而 `optionalString` 不接受 `null`，因此 `api.updateModels()` 在 decoder 边界抛出 `ContractError`。`ModelsModal.saveLocal()` 在 decoder 抛错后提前退出，后续 `api.expose()` 永远不会执行。网关页面重新加载时从后端 canonical config 计算分组，自然仍然是旧的暴露集合。

**证据：** 本轮 `incus` 容器真实 WebUI + Chromium 实测；模型 checkbox 状态已改变，网络中只有 models PUT 200，没有 expose PUT，config 仍为旧 exposure。

**Blocked by:** None — can start immediately

**Status:** ready-for-agent

- [ ] `backup:null` 与 API decoder contract 兼容，models 保存后不会在 decoder 边界失败
- [ ] 模型保存成功后必定执行 expose 保存，且失败时向用户显示明确错误
- [ ] 刷新供应商和网关页面后，新暴露模型仍存在
- [ ] 回归测试覆盖 `backup:null` 响应、模型暴露保存和网关候选列表刷新

## 建议修复方向

优先在 decoder seam 修复 `backup` 的 nullable 语义（或让后端省略 null 字段），然后在正确的 WebUI/API seam 增加回归测试：模拟后端 `backup:null`，确认 `updateModels` 成功返回并继续调用 expose；最后用真实容器 WebUI 重跑勾选→保存→刷新→网关检查。

## Comments

- 相关实现：`webui/src/apiSchema.ts`、`webui/src/api.ts`、`webui/src/components/ProfilesPanel.tsx`、`internal/server/server.go`。
- 关联现象：网关页保存后显示旧模型；本轮直接网关 publish 已单独通过，故该现象主要由暴露集合未持久化造成。
- 本机 `agent-browser` 复测：真实 Chromium 打开 `http://127.0.0.1:46882`，清空 network log 后点击模型窗口「保存」，只观察到 `PUT /api/profiles/mock/models` 返回 200，未观察到 `PUT /api/profiles/mock/expose?channel=main`；刷新后 `model-b` 仍未进入 canonical `exposedModels`。本次没有使用 `/api/*` route mock。
