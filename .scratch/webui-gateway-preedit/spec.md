# Spec: WebUI 网关预编辑（Gateway Pre-edit）

Status: ready-for-agent

## 背景

点击 webui 任意会触发 `sync_gateway_to_pi` 的操作（保存 profile、保存设置、切换 profile、expose/models 更新、failover）时，`~/.pi/agent/models.json` 中 `providerPrefix`（默认 `pi-switch`）网关条目被整体重建，丢失用户手写的额外字段（`headers`、`compat`、`cost`、`modelOverrides`、以及任意 `extra`）。用户期望在 webui 点击“应用/保存”前可预编辑最终写入的 gateway JSON，确认后再落盘。

## 目标

- 任何触发网关同步的操作在真正写入 `models.json` 前弹「预编辑预览」：左 Diff（Current vs Proposed）、右可编辑 JSON 文本框，支持冲突高亮与 JSON 合法性校验。
- 手写额外字段默认保留（手写优先），冲突项高亮提示将被覆盖的键，不强制丢弃。
- 作用范围：全部同步操作（`upsert_profile`、`update_exposed_models`、`update_provider_models`、`set_profile_spoof`、`update_settings`、`set_failover`、`use_profile`）。实现上以后端预览/应用接口为统一出口，前端各面板统一拦截 Save → 预览 → 确认 → 落盘。

## 非目标

- 不接管非网关 provider 的编辑（仅网关 `providerPrefix`）。
- 不提供 models.json 完整文件的自由编辑（仅网关条目）。

## 交互

1. 用户在任意面板点击保存/应用。
2. 前端先调用 `GET /api/models/gateway/preview`（dry-run，不写盘）获取 `{ current, proposed, conflicts }`；若无 current 视为新建。
3. 弹 `GatewayPreviewModal`：左栏按 key 展示 diff（新增/删除/修改，高亮 conflicts），右栏为可编辑 JSON（初始值 = `proposed` 的 pretty JSON）。
4. 用户可直接编辑 JSON；编辑时实时校验 JSON 合法性与 `gateway` 最小 schema（`api`、`baseUrl`、`models` 必填，`api` 在支持列表内）；不合法时禁用确认并提示错误行。
5. 点击「确认应用」→ `PUT /api/models/gateway` 携带编辑后 JSON 落盘（原子写）；取消则不写盘，回到原面板。
6. 对于需要同时修改 `config.json` 的操作（profile/settings/failover 等），流程为：先走预览并确认 gateway → 再执行原 config 写入（仍由原接口完成），两步事务失败任一步回滚提示（gateway 已写则提示可通过备份恢复）。

## 后端

- 新增 `src-rust/ops.rs` 能力：
  - `read_gateway() -> (Option<Value>, Value)` 读取当前 gateway 与计算 proposed（复用现有构建逻辑，但不再丢弃 current 的 extra）。
  - `preview_gateway() -> { current, proposed, conflicts }` dry-run，不写盘。
  - `apply_gateway(Value) -> Result<()>` 校验并原子写入 models.json（保留非网关 providers）。
  - 修改 `sync_gateway_to_pi`：以 `current` 的顶层扁平字段与 `extra` 合并到 `proposed`（手写优先）；`models` 仍由 config 的 exposed 模型权威生成，但每条 model 的 `extra`/`headers`/`compat`/`cost` 若在 current 中存在同 id 则保留并提示 conflict。
- 新增路由 `src-rust/web.rs`：`GET /api/models/gateway`、`GET /api/models/gateway/preview`、`PUT /api/models/gateway`。
- `service.rs` 薄封装 `gateway_state()`、`gateway_preview()`。
- 保留现有 `ops` 变更接口行为不变，仅在 web 前端通过新预览链路调用；CLI/TUI 仍走原直接同步路径（不强制预览）。

## 前端

- `webui/src/api.ts` 新增 `getGateway`、`previewGateway`、`applyGateway`。
- `webui/src/lib/gatewayDiff.ts`：`diffGateway(current, proposed) -> { added, removed, changed }`、`detectConflicts`、`validateGatewayJson`。
- `webui/src/components/GatewayPreviewModal.tsx`：Modal 形态，左右分栏，支持大 JSON 滚动与错误定位。
- 各面板接入：
  - `ProfilesPanel` 的 `ProfileForm.save()` 与 `ModelsModal.save()`、`ProxyPanel.FailoverEditor`、`SettingsPanel`、`useProfile` 均改为：`previewGateway()` → `GatewayPreviewModal` → `applyGateway(edited)` → 原 mutation。

## 验收标准

- [ ] 任意保存/应用操作均先弹预览，取消则不改 `models.json`。
- [ ] 预览左 Diff 正确展示新增/删除/修改，且冲突 extra 字段高亮。
- [ ] 右侧 JSON 可编辑，不合法 JSON 时确认禁用并提示。
- [ ] 确认后 `models.json` 网关条目为编辑后内容，非网关 providers 未被改动。
- [ ] 手写 extra（如 gateway 顶层 `headers: {x-custom: "1"}`、某 model 的 `cost`/`compat`）在无编辑时自动保留。
- [ ] 后端 `GET /preview` 为 dry-run，不产生备份与不写盘。
- [ ] 前端 `vitest` 与后端 `cargo test` 全绿。
