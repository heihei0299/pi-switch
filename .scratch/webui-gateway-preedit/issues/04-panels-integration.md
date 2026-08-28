# 04 面板接入预览

Status: ready-for-agent

Seam: ProfilesPanel / SettingsPanel / ModelsModal / ProxyPanel.FailoverEditor / useProfile 全部 Save 链路

输入：用户点击保存

输出：先 preview → 弹 GatewayPreviewModal → 确认后 applyGateway + 原 mutation，取消则不写盘

验收：
- 任一面板取消预览不改 models.json
- 确认后网关为编辑后内容
- 失败时 toast 提示且不丢失用户编辑
