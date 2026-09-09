# 01 - 配置：Channel 级 API 字段与迁移校验

Status: resolved
Scope: `internal/config` 数据面
Spec: `.scratch/per-channel-api/spec.md`

## 背景
- `Upstream` 无 `api`，`oc` 混布 `chat`/`responses` 模型无法区分，导致 `PlanRequest` 错转 500。

## 交付
- `internal/config/config.go: Upstream` 新增 `API string/json:"api,omitempty"` 与 `ResponsesMode string/json:"responsesMode,omitempty"`，缺省继承 `Profile.API/ResponsesMode`
- `LoadConfigAtPath/MigratedForSave` 兼容旧配置（无 `api` 回退继承），`DefaultConfig` 保持单 `test-provider`
- `MigrateTopLevelToFirstChannel` 保持，若旧配置仅顶层单 `Profile` 且无分区，则首次保存不强制拆分（由用户或后续 `02` 按模型启发式拆分，或保持单通道手工拆）
- 新增 `validateUpstreamAPI`（复用 `ValidateResponsesMode` 规则）：`api` 非法或 `responsesMode` 与 `api` 不兼容时 `handleValidate` 产 `level:error, path: profiles.<name>.upstreams[<idx>].api`
- 单测：旧配置无 `api` 可加载且保存后保留继承；非法 `api` 触发 `validate` 报错

## 验收
- 含 `upstreams[].api` 的新配置 `Load → Save → Load` 往返一致，`api` 落盘
- 旧配置（无 `upstreams[].api`）可加载，`GET /api/validate` 无错，`PUT /profiles/:name` 带 `api` 后 `validate` 按规则报错/通过
- `go test ./internal/config -run TestChannelAPI -count=1` 通过

## 不做
- 不改 `ModelEntry`；不做 `per-conversation` 熔断；不改 `retry.go`
## 实施总结
- 提交：`b39a9ac` — `feat(config): per-channel api fields with validation (01)`
- 实现的 seams：S1 配置 Load/MigratedForSave/validateUpstreamAPI
- 验收标准：
  - [x] 含 upstreams[].api 的新配置 Load→Save→Load 往返一致
  - [x] 旧配置无 api 可加载且 validate 无错
  - [x] 非法 api/responsesMode 触发 validate error
  - [x] go test ./internal/config -run TestChannelAPI 通过
  - [x] go vet 通过
- 测试结果：go test ./internal/config 5/5 PASS，go test ./... 9/9 PASS
- typecheck：通过
- 文档对齐：CONTEXT.md 供应商/渠道定义保持，ADR 0008 分区语义扩展
- 遗留：无
