# 04 面板接入预览

Status: resolved

Seam: ProfilesPanel / SettingsPanel / ModelsModal / ProxyPanel.FailoverEditor / useProfile 全部 Save 链路

输入：用户点击保存

输出：先 preview → 弹 GatewayPreviewModal → 确认后 applyGateway + 原 mutation，取消则不写盘

验收：
- 任一面板取消预览不改 models.json
- 确认后网关为编辑后内容
- 失败时 toast 提示且不丢失用户编辑

## 实施总结
- 提交：`13e4bd2` — `feat(webui): add gateway pre-edit preview to prevent models.json overwrite`
- 实现的 seams：后端预览/应用 + 前端 diff + 预览弹窗 + 面板接入
- 验收标准：
  - [x] preview 不写盘且返回 current/proposed/conflicts
  - [x] apply 校验并原子写入，仅替换 gateway
  - [x] sync 保留手写 extra
  - [x] 前端 diff/校验/弹窗 可编辑且非法时禁用确认
  - [x] 全部保存链路经预览，取消不写盘
- 测试结果：Rust 212 passed, webui 114 passed
- typecheck：通过 (cargo check + tsc --noEmit)
- 文档对齐：WEBUI_GUIDE 已更新 REST map 与 gateway pre-edit 章节
- 遗留 / 后续建议：pending-aware preview（基于待提交 profile 的 proposed）可进一步做 POST preview；per-model conflicts 细化
