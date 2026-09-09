# 02 — 代理启动健康化与不一致提示

**What to build:** 代理启动不再自动同步网关，改为健康检查与不一致提示；用户可在启动后通过健康与预览获知待发布状态，首屏不一致仅提示不自动写。

**Blocked by:** 01 — 去除供应商与设置变更的自动网关写

**Status:** resolved

- [x] 代理与 Web 服务启动时不自动写网关文件，失败仅告警不阻断服务
- [x] 健康检查独立可查，返回运行态、模式、是否有模型文件、上次通知时间与上游总数
- [x] 预览能正确反映本地与网关的差异（新增/移除/变更）与待发布数
- [x] 前端首屏检测到不一致时仅横幅提示“是否立即同步”，不自动落盘
- [x] 网关文件缺失或异常时健康与预览仍可用，供应商侧不受影响

## 实施总结
- 提交：`551dacc` — `feat(supplier-gateway): 代理启动健康化与不一致提示 (#02)`
- 实现的 seams：
  - T1 代理/Web 启动不自动写网关 — `lib.rs:run_proxy_server`/`run_web_server` 改 `gateway::start_placeholder()` 仅 warn 不阻断（输入：启动服务，预期：models.json 未改、notify 触达、health 可查）
  - T2 健康检查独立 — `GET /api/gateway/health` 返回 `GatewayHealth{running,mode,has_models_file,last_notify,upstreams_total,gateway_id,message}`（输入：无，预期：200 + 全字段）
  - T3 预览 pending_count — `gateway::preview_gateway` 计算 `pending_count=added+removed+changed` 并透传 `service::gateway_preview`/`web.rs`（输入：config 变更，预期：current 不变、proposed 变化、pending_count>0）
  - T4 前端首屏横幅 — `GatewayPanel` 首载 `previewGateway` 若 pending>0 则 `setShowMismatchBanner(true)`，仅按钮触发 `applyGateway`，无自动写（输入：首屏加载，预期：横幅显示、可 dismiss、不自动 PUT）
  - T5 异常隔离 — `load_models_value` 缺失返 `{providers:{}}`、health/preview 均 Ok，profiles/gateway 分 Router（输入：删除 models.json，预期：health 200、preview 200、profiles CRUD 仍可用）
- 验收标准：
  - [x] 代理与 Web 服务启动时不自动写网关文件，失败仅告警不阻断服务
  - [x] 健康检查独立可查，返回运行态、模式、是否有模型文件、上次通知时间与上游总数
  - [x] 预览能正确反映本地与网关的差异（新增/移除/变更）与待发布数
  - [x] 前端首屏检测到不一致时仅横幅提示“是否立即同步”，不自动落盘
  - [x] 网关文件缺失或异常时健康与预览仍可用，供应商侧不受影响
- 测试结果：cargo test 299 项全绿；webui vitest 179 项全绿（15 文件）；新增 gateway_preview_pending_count / gateway_start_placeholder_does_not_auto_write 等 5 项
- typecheck：通过（cargo check 通过；webui tsc --noEmit 通过）
- 文档对齐：已更新 README.md / README_ZH.md（启动不再自动写，需显式 Gateway → Apply to Pi）与 WEBUI_GUIDE.md（Gateway explicit publish 小节）
- 遗留 / 后续建议：sync_gateway_to_pi 仍保留供手动调用及未来独立进程使用；前端 pending 计算以 backendPending 为准，diffGateway 仅作回退
