# Spec: 配置 JSON 可编辑 · 接口格式与 .pi/agent/models.json 同步

Status: ready-for-agent

## 背景

pi-switch 的 Profile/Models 目前以表单+卡片结构化编辑为主，用户无法直接粘贴来自 `~/.pi/agent/models.json` 的原始 ModelEntry JSON 进行批量迁移；Gateway 面板与 Profile 配置各自维护一套 Model 视图，字段映射（`contextWindow`/`context_window`、`maxTokens`/`max_tokens`、`cost`、`thinkingLevelMap`、`passthrough`）虽已兼容但未在 UI 层面显式对齐为“与 pi 同格式”，导致用户在两个入口看到的 JSON 形态不一致。需要在保留结构化便利的同时，提供“JSON 可编辑”能力，且保证两处产生的 ModelEntry 与 pi 的 `models.json` 1:1 互通（可直接复制粘贴）。

## 目标

- **配置 JSON 可编辑**：`ProfilesPanel` 的 `ProfileForm` 与 `ModelsModal` 各增加「结构化 / JSON」双 tab，JSON 为主与结构化双向同步（复用 `GatewayPreviewModal` 已验证的模式），校验后显式确认才落盘。
- **接口格式与 `~/.pi/agent/models.json` 相同**：单个 `ModelEntry` 字段与 pi 的 `ModelEntry` 完全一致 — `id`（必填非空）、`name`、`reasoning`、`input`、`contextWindow`/`context_window`兼容、`maxTokens`/`max_tokens`兼容、`thinkingLevelMap`、`cost`（含 `tiers`）、`headers`/`compat` 等，且**未知字段全部透传保留**（`draft.passthrough` 原样回写），保证粘贴 pi 的 model 可无损往返。
- **网关-配置同格式同步**：网关面板与配置文件使用同一套 `piModel` 派生规则（`modelDraft` / `modelPreview` / `buildGatewayPreview`）与同一校验，Profile/Models 保存后网关预览自动刷新（`refresh`），Gateway 的 `Current vs Proposed` 正确反映配置变更，模型增删改在两处视图一致。
- **写入策略**：JSON 语法错误或语义校验失败时行内提示且禁用保存按钮；仅点“保存/确认”才写盘并走现有备份链路。
- **子代理编排与功能测试**：实现过程按 `tdd-implement` 走 seam 级红-绿循环，Code Review 通过子代理执行（受 50 KiB 截断约束，需显式限长），功能测试覆盖 webui `vitest`（`NODE_ENV=test`）与后端 `cargo test`，并补充端到端手动验证（Gateway 发布链路不受影响）。

## 非目标

- 不提供 `~/.pi-switch/config.json` 全文的独立 raw 编辑页（聚焦 Profile/Models 两个入口，已满足批量迁移；如需可在后续迭代补充）。
- 不接管非网关 provider 的顶层 `providers.*` 结构（仅 ModelEntry 层对齐）。
- 不改变后端存储模型（Rust `config::ModelEntry` / `gateway.rs` 的派生逻辑保持不变，仅前端对齐）。

## 交互

### ProfileForm（新增/编辑 Profile）

- 顶部增加 `结构化 | JSON` 切换（默认结构化，保持现有习惯）。
- 结构化视图：保持现有字段（name / apiType / responsesMode / baseUrl / apiKey / headers / compat / upstreams / modelIds 列表）。
- JSON 视图：展示除 `models` 外的 Profile JSON（`api`、`responsesMode`、`baseUrl`、`apiKey`、`headers`、`compat`、`upstreams` 等），pretty 打印 `JSON.stringify(profilePreview, null, 2)`，下方实时校验结果（语法/必填/格式）。切换回结构化时若 JSON 合法则回填结构化状态；非法则保留 JSON 视图并提示，不丢弃编辑。
- 保存：以当前可见 tab 的数据为准；JSON tab 时解析并校验通过后构造 `ProviderProfile` 调 `api.addProfile/updateProfile`，成功后 `toast("已保存") + refresh()`，Gateway 状态自动更新。

### ModelsModal（模型列表）

- 顶部增加 `结构化 | JSON` 切换（默认结构化）。
- 结构化视图：保持现有 `ModelCard` 列表（展开/收起、新增、删除、exposed 切换）。
- JSON 视图：展示 `models: ModelEntry[]` 的 pretty JSON（数组），支持直接粘贴来自 pi 的 `models` 片段。编辑时实时校验每条 `id` 非空、数值字段合法性、`cost` 结构。切换回结构化时按 `id` 稳定 `key` 重建 `ModelDraft`（复用 `GatewayPreviewModal` 的 `prevById` 逻辑）。
- 保存：`api.updateModels(name, models)`，成功后 `refresh()`。

### 校验

- 复用并扩展 `webui/src/lib/gatewayDiff.ts` 的校验思想，新增 `webui/src/lib/piModelValidate.ts`（或扩展现有 `gatewayDiff.ts`）：
  - `validateModelEntry(entry): { ok, error }` — `id` 必填非空字符串；`contextWindow`/`maxTokens` 若存在必须为有限正数；`cost` 若存在必须为对象且 `input/output/cacheRead/cacheWrite` 为 number，`tiers` 若存在为数组。
  - `validateModelsJson(text): { ok, error, value }` — JSON 数组，每项走 `validateModelEntry`，兼容 `context_window`/`max_window` 字段的读取但输出统一为 camelCase。
  - Profile JSON 校验：`api` 在支持列表、`baseUrl` 以 http(s) 开头（复用 `validateGatewayJson` 的 SUPPORTED_APIS 逻辑的 Profile 版）。

### 网关-配置同步

- `ProfileForm` / `ModelsModal` 保存成功后调用 `refresh()`（已在 `ProfilesPanel` 中实现），`GatewayPanel` 的 `previewGateway()` 在下一次 `load()` 时自动反映新 `proposed`；不引入 Gateway 的自动写入，仅保证展示一致性。
- `piModel.ts` 的 `CONTROLLED_MODEL_KEYS` 与 `modelDraft`/`modelPreview` 为唯一事实源，Gateway 与 ModelsModal 共享，避免字段漂移。

## 后端

- 无新增路由；复用现有 `PUT /profiles/:name` / `PUT /profiles/:name/models` 与 `GET /models/gateway/preview` 已有能力。
- Rust 侧 `config::ModelEntry` 已支持 `#[serde(flatten)] extra` 透传，前端透传字段经 `extra` 原样落盘，保持兼容。

## 前端改动点

- `webui/src/lib/piModel.ts`：导出 `validateModelEntry` / `validateModelsJson`（或独立文件），保持 `CONTROLLED_MODEL_KEYS` 为 ModelEntry 1:1 依据。
- `webui/src/lib/gatewayDiff.ts`：如复用则扩展，或保持独立校验文件避免耦合。
- `webui/src/components/ProfilesPanel.tsx`：`ProfileForm` 与 `ModelsModal` 各加 `mode: "structured" | "raw"`、`text` 状态、`validation` memo、切换时的双向同步（`useEffect` 监听 `mode`/`text`）。
- `webui/src/i18n`：补充 `结构化` / `JSON` / `JSON valid` / `Invalid JSON` 等文案（复用 Gateway 已有 i18n key 如有）。
- 样式：复用 `GatewayPreviewModal` 的 tab 样式（`border border-white/10 bg-zinc-900 p-0.5 rounded-md`）。

## 验收标准

- [ ] `ProfileForm` 有结构化/JSON 双 tab，JSON 非法时保存禁用且有行内错误提示，合法时切换回结构化数据正确回填。
- [ ] `ModelsModal` 有结构化/JSON 双 tab，JSON 为 `ModelEntry[]` 数组，可粘贴 pi 的 model（含 `cost.tiers`、`thinkingLevelMap`、未知字段），保存后数据往返无损（未知字段保留）。
- [ ] `contextWindow`/`context_window`、`maxTokens`/`max_tokens` 双向兼容，输出统一为 `contextWindow`/`maxTokens`。
- [ ] Profile JSON 校验覆盖 `api` 支持列表与 `baseUrl` http(s) 前缀。
- [ ] Models JSON 校验覆盖 `id` 非空、`contextWindow`/`maxTokens` 正数、`cost` 结构。
- [ ] 保存后网关 `Current vs Proposed` 状态条正确更新（`refresh` 后 `previewGateway` 反映新模型），无需手动刷新页面。
- [ ] 既有结构化编辑链路不受影响（不改字段时保存结果与改动前一致）。
- [ ] `NODE_ENV=test npx vitest run` 与 `cargo test` 全绿，新增测试覆盖 JSON 往返、校验、透传保留。
- [ ] Code Review 通过子代理执行且输出受控（≤400 words / ≤40 行，含截断说明）。

## 子代理编排

- 实现阶段每 seam 一个红-绿循环，一回合内连续完成（红→绿→typecheck），中间不交付。
- 需求澄清与 seam 确认由主代理完成；实现与测试由主代理直接执行，必要时派 `Explore` 子代理（read-only）做代码定位，不派实现类子代理以避免 50 KiB 截断与 session 隔离问题。
- Code Review 阶段派 `code-review` 子代理，`task` 必须显式含「输出 ≤400 words / ≤40 行」限长（项目硬性约束，见 `project-memory #23`）。
- 功能测试由主代理在本地执行（`vitest` / `cargo test`），子代理不承接测试执行。
