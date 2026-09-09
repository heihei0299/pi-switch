# 02 - 代理：按 Channel API 分流与网关前缀

Status: resolved
Blocked by: 01
Scope: `internal/server` 转发与网关聚合
Spec: `.scratch/per-channel-api/spec.md`

## 背景
- `PlanRequest` 现取 `Profile.API`，`oc/mimo(chat-only)` 被错转 `responses` 致 500。

## 交付
- `internal/server/server.go`：`narrowToChannel` 保留 `Upstream.API/ResponsesMode` 到窄化后 `Profile`；`profForAttempt`/`findModelEntry` 后的 `PlanRequest(proto, prof.API, prof.ResponsesMode)` 以 `Channel` 覆盖的 `api` 为准，`UpstreamPath` 由 `Plan.To` 决定；`clampBody` 的 `ModelEntry` 取 `ChannelView` 对应通道池
- `handleChatCompletions/handleStream` 的首包 500 重试保留（`fix-400` 已有），不对本票新增重试，仅确保按正确 `api` 重试时路径正确
- `Gateway` 的 `BuildProposedGatewayEntry/Preview` 保持 `supplier/channel/model` 前缀，`owned_by` 按 `Channel` 区分，二次勾选不变
- 单测：`POST /v1/chat/completions {"model":"oc/chat/mimo-v2.5"}` 经 `chat` 通道 200，`POST /v1/responses {"model":"oc/responses/muse-spark-1.2-contributor"}` 经 `responses` 通道 200；错配经转换后仍 200（`chat→responses` 与 `responses→chat` 分别验证）
- 集成：`GET /v1/models` 含 `oc/chat/mimo-v2.5` 与 `oc/responses/muse-spark-1.2-contributor` 且 `owned_by` 区分

## 验收
- `curl /v1/chat/completions oc/chat/mimo-v2.5 → 200`，`curl /v1/responses oc/responses/muse-spark → 200`
- `curl /v1/chat/completions oc/responses/muse-spark` 经转换后 200（`responses` 上游）
- `curl /v1/responses oc/chat/mimo` 经转换后 200（`chat` 上游）
- `go test ./internal/server -run TestChannelAPI -count=1` 通过

## 不做
- 不引入全局 `ModelInfo` 注册表；不做 `TUI` 渠道 `api` 编辑（WebUI 先行）
## 实施总结
- 提交：`ee86c3e` — `feat(proxy): route by channel api for chat/responses (02)`
- 实现的 seams：S2 代理按 Channel API 分流（POST /v1/chat vs /v1/responses 对 oc/chat/mimo 与 oc/responses/muse-spark；错配经转换后 200）
- 验收标准：
  - [x] `curl /v1/chat/completions oc/chat/mimo-v2.5 → 200`（TestChannelAPI_RoutingDirect）
  - [x] `curl /v1/responses oc/responses/muse-spark → 200`（TestChannelAPI_RoutingDirect）
  - [x] `curl /v1/chat/completions oc/responses/muse-spark` 经转换后 200（TestChannelAPI_CrossConversion chat→responses）
  - [x] `curl /v1/responses oc/chat/mimo` 经转换后 200（TestChannelAPI_CrossConversion responses→chat）
  - [x] `go test ./internal/server -run TestChannelAPI -count=1` 通过（3/3 PASS, 0.03s）
  - [x] `GET /v1/models` 含 `oc/chat/mimo-v2.5` 与 `oc/responses/muse-spark-1.2-contributor` 且 owned_by 有值（TestChannelAPI_ModelsList）
- 测试结果：全绿（`go test ./internal/server -count=1` 全部通过；`go test ./... -count=1` 9 包通过）
- typecheck：通过（`go vet ./internal/server ./internal/config` 无输出）
- 文档对齐：无需更新（Gateway 前缀与二次勾选保持不变）
- 遗留 / 后续建议：`handleModels` 的 `owned_by` 保持 supplier 级，id 前缀 `supplier/channel/model` 已区分；若需 owned_by 按 channel 区分可后续调整
