# 01 — 去除供应商与设置变更的自动网关写

**What to build:** 供应商个性与网关全局设置变更后不再自动写网关，仅在网关发布时落盘；用户在供应商与设置界面保存后得到本地保存提示，网关需到发布面板显式发布。

**Blocked by:** None — can start immediately

**Status:** resolved

- [x] 供应商个性变更后网关文件未被改写，预览 pending 如期增加但 current 未变
- [x] 设置变更（gatewayApi/host/port/providerPrefix）后网关文件未被改写，仅产生待发布差异
- [x] 已废弃的代理目标设置变更同样不自动写网关
- [x] 前端在供应商与设置保存后仅提示“已保存到本地，需到网关发布”，不触发预览或发布弹窗
- [x] 供应商侧校验或落盘失败不影响网关读与预览，反之亦然（本票范围内验证单向）
- [x] 存量单字段与多上游回退兼容保持，上游空列表不落盘，未知字段透传不提权

## 实施总结
- 提交：`d528cda5708519774c5cec3648f38d93ba25ef85` — `feat(supplier-gateway): 去除自动网关写 (#01)`
- 实现的 seams：
  - T1 `PUT /profiles/:name/spoof` 供应商个性变更不自动写网关（输入：spoof preset，预期：config 落盘成功、models.json current 不变、preview pending 增加、notify 未触发）
  - T2 `PUT /settings` 网关全局设置（gatewayApi/host/port/providerPrefix）变更不自动写网关，仅 preview proposed 变化（输入：全量 settings JSON，预期：config 落盘、models.json 未改、preview current!=proposed）
  - T3 `ops::set_proxy_target` 已废弃代理目标设置变更不自动写网关（输入：target Option<&str>，预期：仅写 config.json，不触 models.json/notify）
  - T4 前端 `ProfilesPanel`/`SettingsPanel` 保存仅提示“已保存到本地，需到网关发布”，不触发 previewGateway/applyGateway 弹窗（输入：用户点击保存，预期：调 updateSettings/updateProfile，仅 toast 提示，无 GatewayPreviewModal）
  - T5 单向错误隔离：供应商侧校验/落盘失败不影响网关读/预览（输入：非法 profiles payload 导致 400，预期：GET /models/gateway 预览仍 200）
  - T6 兼容性保持：多上游回退、空列表不落盘、未知字段透传不提权（输入：含 upstreams/extra 的 ProviderProfile，预期：resolved_upstreams 回退、上游空列表 skip_serializing、extra 原样保留）
- 验收标准：
  - [x] 供应商个性变更后网关文件未被改写，预览 pending 如期增加但 current 未变
  - [x] 设置变更（gatewayApi/host/port/providerPrefix）后网关文件未被改写，仅产生待发布差异
  - [x] 已废弃的代理目标设置变更同样不自动写网关
  - [x] 前端在供应商与设置保存后仅提示“已保存到本地，需到网关发布”，不触发预览或发布弹窗
  - [x] 供应商侧校验或落盘失败不影响网关读与预览，反之亦然（本票范围内验证单向）
  - [x] 存量单字段与多上游回退兼容保持，上游空列表不落盘，未知字段透传不提权
- 测试结果：cargo test 290 项全绿；webui vitest 179 项全绿（15 文件）；新增 tdd_spoof_does_not_trigger_gateway_write、tdd_settings_does_not_trigger_gateway_write、SettingsPanel 3 项解耦测试
- typecheck：通过（cargo check 通过；tsc --noEmit 仅遗留 index.css 侧效应导入提示，无实际类型错误）
- 文档对齐：无需更新（README 已描述 Profiles 仅写本地、网关经 Current vs Proposed/Apply to Pi 显式发布，与实现一致；ADR-0006 已新增并与实现一致；CONTEXT.md 已补充供应商/上游/网关/网关发布术语）
- 遗留 / 后续建议：Proxy 启动时 `lib.rs::run_proxy_server` 仍保留 `sync_gateway_to_pi()` 预发布（属后续启动健康检查 issue 范畴，本票未改）；FailoverEditor 已解耦预览但保留“Failover saved”提示，如需统一为隔离提示可后续对齐
