# 08: API 合约与校验 responsesMode/validate

**What to build:** 让 `webui/src/types.ts` 与 `Go gin json tag` 的手写同步被契约测试守护，`responsesMode` 不兼容在保存时 `400`，`validate` 仅提示不阻塞，`failover` 热更新可被 `httptest` 断言。

**Blocked by:** 07 构建与嵌入基座与 buildInfo 可观测性

**Status:** resolved

- [x] `webui/src/types.ts`（`ProviderProfile/ModelEntry/Settings/ResponsesMode`）与 `internal/config:ProviderProfile/ModelEntry/Settings` 的 `json` tag 手动同步，`types.ts` 顶部注释指向 `config.go` 为源真，`go test` 以 `gateway publish` 写前写后差异间接回归漂移（不引入 OpenAPI 生成）
- [x] `POST /api/profiles` 与 `PUT /api/profiles/:name` 及 `PUT /api/config` 的原始 json 预检 `validateProfileResponsesMode`：`passthrough` 仅允许 `api=openai-responses`，`convert` 仅允许 `api=openai-completions`，否则 `400 {"error":"responsesMode passthrough requires api openai-responses, got ..."}`；`auto` 恒过
- [x] `GET /api/validate | GET /api/config/validate` 返回 `ValidationIssue[]{level,path,message}`（`warning: no models / modelsDevProvider unknown / failover not found` 含 `failover` 热更新），不作为 `PUT` 前置阻塞（先保存再 `gateway publish` 时校验）
- [x] `404 for unknown profile` 与 `500 for upstream` 保持分散（`GET /api/profiles/:name` 与 `POST /api/profiles/:name/fetch-models` 的 `httptest.Server` 上游错误分别断言）
- [x] `validateProviderProfile: duplicate model id / exposedModels references unknown model → 400`，与 `modelsDevProvider` 的 `warning` 分级在 `go test` 覆盖

## 实施总结
- 提交：`fcd716e` — `feat(rewrite-go-increment): api contract validate (#08)`
- 实现的 seams：S1 types.ts 手写同步, S2 responsesMode 400, S3 validate 仅提示, S4 profile 校验 400
- 验收标准：5/5 全选
- 测试结果：go test ./internal/server -run TestApiContract_08 5/5 全绿；go test ./... 全绿；go vet 通过
- typecheck：通过
- 文档对齐：webui/src/types.ts 头部已更新；internal/config/config.go 新增 modelsDevProvider
- 遗留 / 后续建议：modelsDevProvider 白名单 11 项，后续可联动 models.dev 自动校验
