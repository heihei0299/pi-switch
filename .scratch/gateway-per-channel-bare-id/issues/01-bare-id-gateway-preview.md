# 01: 裸 ID 网关预览

**What to build:** 在 `供应商` 的某 `渠道` 勾选已暴露模型后，`Gateway` 预览按 `渠道` 分 provider 展示裸 `id`，每 provider 的 `api` 取该 `渠道` 的 `api`，`pending` 与 `模型目录` enrich 按 `providerKey+modelId` 对比与查询，用户可在 GatewayPanel 看到按 `渠道` 分组的裸 `id` 预览。

**Blocked by:** None (can start immediately)

**Status:** resolved

- [x] `BuildProposedGatewayEntry` 按 `Channel` 生成 `providers[supplier/channel]` 多条目，`models[].id` 为裸 `id`
- [x] `POST /models/gateway/preview` 返回多 provider 形态，`pending_count` 与 `enrich` 按 `渠道` 循环
- [x] WebUI GatewayPanel 预览按 `Supplier→Channel` 树展示裸 `id`，无斜杠解析
- [x] `gateway_test` 覆盖多渠道分 provider 与裸 `id`，`handleGatewayPreview` 单测绿

## 实施总结
- 提交：`b9be77e` — feat(gateway-per-channel-bare-id): 01 bare-id gateway preview
- 实现的 seams：BuildProposed裸ID多Provider、Preview/Enrich/Pending按providerKey、GatewayPanel树裸ID（groups已按渠道，pending双层）
- 验收标准：
  - [x] BuildProposed按Channel生成providers[supplier/channel]多条目，models[].id裸
  - [x] POST /models/gateway/preview返回多provider形态，pending与enrich按渠道循环
  - [x] WebUI GatewayPanel预览按Supplier→Channel树展示裸id（groups，pending双层）
  - [x] gateway_test覆盖多渠道分provider与裸id，handleGatewayPreview单测绿
- 测试结果：go test ./... 9 passed, webui 240 passed
- typecheck：通过
- 文档对齐：无（CONTEXT/ADR延至04）
- 遗留/后续建议：api回退与legacy短key保留至04清理；WebUI slash清理延至03
