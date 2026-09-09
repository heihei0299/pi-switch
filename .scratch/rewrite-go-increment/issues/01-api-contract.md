## Question

Go 后端与 React 前端的 API 合约应以何种机器可验证形式对齐？如何定义错误码与校验细节（`responsesMode` 400 约束、`validate` 的 `level/path/message`、`gateway publish` 的写前写后一致性）以避免 `new-provider` 残留与 `3 vs 2` 模型的契约漂移？

需决策：
1. 合约形式：手写 `webui/src/types.ts` 与 `internal/server` 的 `gin` 路由的 `json` tag 手动同步 vs 生成 `OpenAPI` + `openapi-typescript` / `oapi-codegen` 的强校验
2. 错误码：`400` 仅用于 `responsesMode` 不兼容（`passthrough↔openai-responses`/`convert↔openai-completions`）与 `validate` 的 `level/path/message`，其余错误码（`404` for unknown profile, `500` for upstream）是否统一
3. 校验：`validate_config` 的 `level/path/message` 是否作为 `PUT /api/profiles` 的前置阻塞，还是仅 `GET /api/validate` 的提示
4. 网关发布的契约：`PUT /api/gateway/publish` 的写前（`providers:{}`）与写后（含 `pi-switch`）的 `models.json` 差异如何作为契约测试的断言

## Type

grilling

## Status

resolved

## Answer

1. **合约形式**：手写 `webui/src/types.ts` 与 `internal/server` 的 `json` tag 手动同步，辅以 `go test` 的契约断言（`gateway publish` 写前写后差异），不引入 `OpenAPI` 生成管线。
2. **错误码**：`400` 仅用于 `responsesMode` 不兼容（`passthrough↔openai-responses`/`convert↔openai-completions`）与 `validate` 的 `level/path/message`，`404` for unknown profile / `500` for upstream 保持分散。
3. **校验**：`validate_config` 仅 `GET /api/validate` 提示，`PUT /api/profiles` 不前置阻塞，允许先保存再 `gateway publish` 时校验。
4. **网关契约**：`PUT /api/gateway/publish` 契约测试断言写前 `providers:{}` 或缺 `pi-switch`，写后 `providers[pi-switch]` 含 `exposedModels` 过滤后的 2 模型（回归 `3 vs 2`），`preview pending` 的 `diff` 亦纳入断言。
