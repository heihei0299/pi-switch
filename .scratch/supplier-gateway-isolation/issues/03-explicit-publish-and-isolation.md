# 03 — 显式发布闭环与双向错误隔离加固

**What to build:** 网关发布成为唯一写入口并完成端到端闭环，供应商与网关在接口与界面上双向错误隔离；用户在网关面板查看差异并显式发布，任一侧故障不串扰另一侧。

**Blocked by:** 01 — 去除供应商与设置变更的自动网关写, 02 — 代理启动健康化与不一致提示

**Status:** resolved

- [x] 预览为干跑不写盘，发布为原子写并通知，校验失败返回 400 且不影响供应商侧
- [x] 供应商路由错误不影响网关读/预览/健康，网关路由错误不影响供应商增删改查
- [x] 网关面板展示 Current vs Proposed、待发布数与冲突列表，发布成功清 pending 且刷新
- [x] 发布失败保留编辑态可重试，不丢失待发布内容
- [x] 前端错误边界隔离：网关离线时供应商面板仍可正常增删改
- [x] 供应商为唯一权威，网关只读派生，合并时仅透传未知字段不提权

## 实施总结
- 提交：`d97ce09` — `feat(supplier-gateway): 显式发布闭环与双向隔离加固 (#03)`
- 实现的 seams：
  - S1 `PREVIEW_DRY_RUN` — GET /models/gateway/preview 两次调用一致且不改 models.json (pending_count/conflicts/current vs proposed)
  - S2 `APPLY_VALIDATION_400_ISOLATED` — PUT /models/gateway 非法体 →400 且 models.json 未动，GET /state 与预览仍 200
  - S3 `APPLY_ATOMIC_NOTIFY` — PUT 合法 proposed →200，原子写盘 + notify，preview pending 清零且 current 有值
  - S4 `BIDIRECTIONAL_ROUTER_ISOLATION` — profiles 400 不阻断 gateway health/preview；gateway 400 不阻断 profiles CRUD（make_profiles_router / make_gateway_router 独立）
  - S5 `GATEWAY_PANEL_UI_CLOSED_LOOP` — Current vs Proposed 状态条、待发布数、冲突列表、lastPublishAt；成功刷新 preview + refresh 回调
  - S6 `PUBLISH_FAILURE_RETAIN_DRAFT` — apply 失败 toast 且不 reload，draft 与 pending 保留可重试
  - S7 `FRONTEND_BOUNDARY_AND_AUTHORITY` — PanelErrorBoundary 隔离，gateway 离线时 supplier 仍可用；merge_gateway_extra 仅透传未知字段不提权
- 验收标准：
  - [x] 预览为干跑不写盘，发布为原子写并通知，校验失败返回 400 且不影响供应商侧
  - [x] 供应商路由错误不影响网关读/预览/健康，网关路由错误不影响供应商增删改查
  - [x] 网关面板展示 Current vs Proposed、待发布数与冲突列表，发布成功清 pending 且刷新
  - [x] 发布失败保留编辑态可重试，不丢失待发布内容
  - [x] 前端错误边界隔离：网关离线时供应商面板仍可正常增删改
  - [x] 供应商为唯一权威，网关只读派生，合并时仅透传未知字段不提权
- 测试结果：cargo test 304 项全绿；webui vitest 183 项全绿（16 文件）；新增 gateway_merge_extra_does_not_elevate_generated_keys、gateway_apply_is_atomic_and_notifies、gateway_apply_validation_400_does_not_touch_models_and_supplier_isolated、gateway_apply_success_is_atomic_and_clears_pending、profile_error_does_not_block_gateway...、GatewayPanel 冲突/离线 2 项、SupplierGatewayIsolation 2 项
- typecheck：通过（cargo check 无错误；webui tsc --noEmit 无错误；vite build 成功）
- 文档对齐：无需更新（README 已含“Profiles only write local config, Gateway explicitly publishes via Current vs Proposed & Apply to Pi”；WEBUI_GUIDE 已含 Gateway explicit publish 小节；ADR-0006 已描述双向隔离与唯一写入口，与实现一致）
- 遗留 / 后续建议：sync_gateway_to_pi 仍为保留接口供未来独立进程手动调用；前端 lastPublishAt 仅本地 localStorage，未持久到后端；后续真拆进程时可将 gateway.notify 文件通道替换为 IPC
