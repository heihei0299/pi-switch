# 02: 按渠道发布与代理裸 ID 路由

**What to build:** 用户在预览后点 `应用到 Pi`，`models.json: providers` 全量覆写为按 `渠道` 分条目的裸 `id` 列表；代理按裸 `id` 全局扫描所有 `渠道` 的 `exposedModels`，唯一命中则透传，`GET /v1/models` 按 `渠道` 分组返回裸 `id`，`curl` 可验证端到端。

**Blocked by:** 01: 裸 ID 网关预览

**Status:** resolved

- [x] `Publish` 全量覆写 `providers`（不增量 merge），旧 `providers` 清空
- [x] `resolveRoute` 裸 `id` 全局扫描，0→`502 no_route`，1→命中并窄化到该 `Channel`，>1→`502 ambiguous`
- [x] `handleModels` 按 `Channel` 分组返回 `owned_by=providerKey` 裸 `id`
- [x] `passthrough` 单测与 `handleModels` 单测覆盖唯一/歧义裸 `id` 路径

## 实施总结
- 提交：`ec12170` — feat(gateway-per-channel-bare-id): 02 per-channel publish and bare route
- 实现的 seams：Publish全量覆写、resolveRoute全局扫描、handleModels分组裸ID、透传窄化
- 验收标准：
  - [x] Publish全量覆写providers旧清空
  - [x] resolveRoute裸全局0/1/>1
  - [x] handleModels按Channel分组裸id owned_by providerKey
  - [x] passthrough单测与handleModels单测覆盖
- 测试结果：go 9 / webui 240, handleModels bare, resolveRoute global, passthrough 429 isolated
- typecheck：通过
- 文档对齐：无
- 遗留/后续建议：斜杠400与WebUI分组延至03；api回退与legacy短key延至04
