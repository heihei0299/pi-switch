# 0009 网关按渠道分 provider 与裸模型 ID

日期: 2026-09-08
状态: 已接受
关联: ADR-0008（渠道分区模型与网关二次勾选）、ADR-0006（供应商与网关彻底逻辑独立）

## 背景

ADR-0008 为保跨渠道同 `modelId` 唯一，将 `网关` 模型 `id` 定为 `supplier/channel/modelId` 并聚合进单 provider。该前缀泄露内部路由键到身份，导致 WebUI 需在多种 id 形态间反复解析；同时单 `GatewayAPI` 统管所有模型，迫使 `Responses` 与 `Chat` 模型混在同一 provider 下依赖自动转换。

参照 `CLIProxyAPI`：模型 `id` 保持裸名（`alias`），隔离由多 provider 条目承载，每 provider 独立声明 `api/format`，调度按裸 `id` 选凭据，歧义时才要求显式 `prefix`。

## 决定

1. **裸 `id`**：`网关` 落盘 `models[].id` 仅为裸 `modelId`，不含 `supplier/channel` 前缀；跨 `渠道` 同 `modelId` 视为同一逻辑模型的多次暴露。
2. **按 `渠道` 分 provider**：`BuildProposedGatewayEntry`/`Publish` 按每 `渠道` 生成独立 provider 条目 `providers[supplier/channel]`，`api` 必须取 `Channel.api`，`models` 仅含该 `渠道` 的裸 `id` 列表。`providers` 的 key 即源标识，无需在 `id` 内编码。
3. **严格不兼容**：代理侧 `POST /v1/chat/completions|/v1/responses` 的 `body.model` 含 `/` 直接 `400`，不再接受 `supplier[/channel]/model` 前缀；裸 `id` 多处暴露时报 `502 ambiguous`，由调用方消解。存量 `models.json` 仍按新 provider wrapper 读取，旧 flat 或带 `/` 的模型 id 不做剥离，下一次发布会覆盖为新结构。
4. **术语更新**：`CONTEXT.md: 网关/网关发布/渠道` 同步改写为上述定义。

## 备选

- 保留单 provider 仅改 `id` 为裸名：写法简单但 `api` 仍需单值，无法按 `渠道` 暴露正确 `api`，否决。
- 保留前缀但在 WebUI 隐藏：仅掩盖显示，未解决 `id` 既身份又路由的耦合与 `Chat↔Responses` 混卖，否决。
- 裸 `id` 歧义时池化轮询（CPA 风格）：需引入调度器与策略，与 `pi-switch` 现有 `Current` 单候选纪律冲突，否决；二階段校验留待后续。

## 后果

- 正面：`id` 无斜杠，WebUI 无需解析前缀；每 `渠道` 的 `api` 正确，`Responses` 与 `Chat` 模型分 provider 暴露，无需自动跨协议转换即可正确透传。
- 负面：含 `/` 的旧 `model` 参数与已发布 `models.json` 的旧 `id` 立即失效，需一次性重新 `网关发布`；旧脚本需改裸 `id`。
- 迁移：`config.json` 的模型池与暴露集继续按 channel 保存；`Channel.api` 为必填。旧 `models.json` 不自动迁移，需在供应商中重新选择 channel/model 后执行一次网关发布。
