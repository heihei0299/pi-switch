# 03 预览弹窗组件

Status: resolved

Seam: `GatewayPreviewModal` React 组件

输入：`current`, `proposed`, `conflicts`, `onConfirm(editedValue)`, `onClose`

输出：左 diff（高亮 conflicts），右可编辑 JSON Textarea，实时校验，确认禁用态，取消关闭

验收：
- 不合法 JSON 时确认 disabled 并显示错误
- 冲突行高亮 amber
- 大 JSON 可滚动不溢出

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
