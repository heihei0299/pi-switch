# 01: 实现固定双 provider 的正常发布路径

**What to build:** 配置包含多个 Supplier/Channel 时，用户可以在 GatewayPanel 中看到目标 provider 与来源分组，勾选 exposed models 并成功发布；最终 pi-switch 只生成 `pi-switch-res` 与 `pi-switch-chat`，两个 provider 的 API contract 正确，模型聚合、元数据、model-level session affinity 和稳定排序均生效。Responses 与 Chat 请求可以分别通过真实 proxy 到达对应上游。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] `openai-responses` 模型进入 `pi-switch-res`，`openai-completions` 模型进入 `pi-switch-chat`。
- [x] 没有 exposed model 的 provider 被省略；正常情况下只生成两个固定 pi-switch provider，且不生成 Supplier/Channel provider。
- [x] 两个固定 provider 的 API contract、proxy 连接信息和模型元数据正确，模型按稳定规则排序。
- [x] opencode 来源模型具有 model-level `sendSessionAffinityHeaders`，普通模型不继承该兼容字段。
- [x] GatewayPanel 同时展示目标 provider 与来源 Supplier/Channel，模型勾选后发送固定 provider key 的发布内容。
- [x] 真实 Responses 与 Chat 请求分别通过 proxy 到达对应真实 Channel。

## 实施总结
- 提交：`3f7807f` — `feat(gateway): publish fixed dual providers`
- 实现的 seams：固定双 provider 聚合发布、preview 目标/来源分组、裸 model id 与稳定排序、model-level session affinity、真实 Responses/Chat 路由。
- 验收标准：以上 6 项全部通过。
- 测试结果：Go 相关包 150 tests passed；WebUI 28 files / 243 tests passed。
- typecheck：通过。
- 文档对齐：README 已改为固定 `pi-switch-res` / `pi-switch-chat` 合约。
- 遗留 / 后续建议：Issue 02、Issue 03 按依赖顺序继续实现。
