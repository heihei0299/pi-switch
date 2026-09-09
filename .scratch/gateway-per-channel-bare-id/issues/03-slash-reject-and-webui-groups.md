# 03: 斜杠硬拒绝与 WebUI 分组落地

**What to build:** 任何含 `/` 的 `body.model` 被代理直接拒绝，WebUI 的网关二次勾选与展示恒按 `Supplier→Channel` 树以裸 `id` 交互，前端彻底无斜杠解析与短显补丁。

**Blocked by:** 02: 按渠道发布与代理裸 ID 路由

**Status:** ready-for-agent

- [ ] `handleChatCompletions/handleStream` 入口 `model` 含 `/` → `400 invalid_request_error: model must not contain "/"`
- [ ] GatewayPanel 二次勾选按 `Channel` 粒度，提交 `providers` map，`gatewayDiff` 按 `providerKey+modelId` 双层对比
- [ ] 删 `webui` 中所有 `split("/")`、三段/二段拼接与隐藏 channel 段逻辑
- [ ] 单测与 `vitest` 覆盖 `400` 分支与按渠道分组展示
