# Spec: 网关按渠道分 provider 与裸模型 ID

Status: ready-for-agent
Slug: gateway-per-channel-bare-id
ADR: docs/adr/0009-gateway-per-channel-bare-id.md
Context: CONTEXT.md#网关, #网关发布, #渠道

## 背景
ADR-0008 的 `supplier/channel/modelId` 前缀将路由键泄露进 `id`，导致 WebUI 三形态解析与单 `GatewayAPI` 下 `Chat↔Responses` 混卖。参照 CPA 的裸 `id` + 多 provider 分离，按 Q1-Q8 共识收敛。

## 目标（落地后）
- `models.json: providers` 恒按 `Channel` 分条目，`key = supplier/channel`（无单渠道简写），每条目 `api = Channel.api`（必填，无回退，不读 `Settings.GatewayAPI`），`models[].id` 为裸 `modelId`。
- 代理侧裸 `id` 全局唯一命中，多处暴露报 `502 ambiguous`，含 `/` 的 `model` 参数直接 `400`。
- WebUI 按 `Supplier→Channel` 树展示裸 `id`，无斜杠解析。
- 全量不兼容：无读时剥离、无自动迁移，存量 `models.json` 旧 `supplier/channel/model` 条目与旧 `body.model` 前缀请求一律视为脏数据丢弃/拒绝，下次 `Gateway Publish` 全量重建。

## 非目标
- 不引入 CPA 的池化调度/权重/粘滞；仍单候选。
- 不改 `Supplier/Channel` 存储结构。

## 范围与文件
- `internal/gateway/gateway.go`: `BuildProposedGatewayEntry`, `gatewayModelEntry`, `Publish`（全量覆写 `providers`，不增量 merge 不备份兼容）, `MergeGatewayExtra`, `ComputePendingCount`
- `internal/server/server.go`: `handleGatewayPreview`, `enrichProposedModels`（按 `providerKey` 循环，不从 `id` 解析 `supplier`）, `resolveRoute`, `handleChatCompletions/handleStream` 含 `/` 400, `handleModels` 按 `Channel` 分组裸 `id`
- `internal/config/config.go`: 删除 `Settings.GatewayAPI` 与 `ProviderPrefix` 单 provider 写入逻辑（`settings` 仅留 `host/port`），删顶层 `Supplier.models/exposedModels` 回退
- `webui/src/components/GatewayPanel.tsx`, `webui/src/lib/gatewayDiff.ts`, `webui/src/api.ts`
- `docs/adr/0009`, `CONTEXT.md` 已更新（后续 ADR 追加不兼容修订）

## 行为契约
1. 发布：`POST /models/gateway/preview` 与 `PUT /models/gateway` 均返回/落盘多 provider 形态 `providers[supplier/channel]`；`pending_count` 按裸 `id` 双层对比（`providerKey + modelId`）。
2. 路由：`body.model` 含 `/` → `400 {"error":{"message":"model must not contain \"/\"","type":"invalid_request_error"}}`；裸 `id` 0 处暴露 → `502 no_route`；>1 处暴露 → `502 ambiguous`。
3. 兼容：全量不兼容，无剥离、无回退、无前缀接受。
4. 迁移：无自动迁移；存量 `models.json` 旧含 `/` 条目与旧脚本前缀请求直接废弃，需手动在 WebUI 按 `Channel` 重新勾选发布一次即重建。

## 验证
- `gateway_test.go`: 多渠道分 provider、恒 `supplier/channel` key、无单渠道简写、无旧前缀剥离
- `server_test.go` / `passthrough_single_test.go`: 裸唯一命中、歧义 502、含 `/` 400、Chat/Responses 按 provider `api` 透传不混转
- WebUI: `GatewayPanel` 按 `Supplier→Channel` 树裸 `id` 展示、二次勾选落盘多 provider
- 手工：`npm run build:webui && go build` 后浏览器验证 `models.json` 多 provider 裸 `id` 与代理 `curl /v1/chat/completions` 三分支；旧 `models.json` 发布前直接丢弃验证
