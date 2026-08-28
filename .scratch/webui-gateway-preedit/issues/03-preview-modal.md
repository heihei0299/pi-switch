# 03 预览弹窗组件

Status: ready-for-agent

Seam: `GatewayPreviewModal` React 组件

输入：`current`, `proposed`, `conflicts`, `onConfirm(editedValue)`, `onClose`

输出：左 diff（高亮 conflicts），右可编辑 JSON Textarea，实时校验，确认禁用态，取消关闭

验收：
- 不合法 JSON 时确认 disabled 并显示错误
- 冲突行高亮 amber
- 大 JSON 可滚动不溢出
