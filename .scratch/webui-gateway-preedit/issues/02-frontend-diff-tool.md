# 02 前端 diff 与校验工具

Status: resolved

Seam: `webui/src/lib/gatewayDiff.ts` 纯函数

输入：current/proposed JSON

输出：
- `diffGateway` → `{ added, removed, changed }` 按顶层与 models 数组分段
- `detectConflicts` → 冲突键列表
- `validateGatewayJson(text) -> { ok, error, value }` 校验 JSON 合法性与最小 schema

验收：
- 非法 JSON 返回 error 含行列提示
- 缺少 api/baseUrl/models 时校验失败

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
