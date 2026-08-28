# 02 前端 diff 与校验工具

Status: ready-for-agent

Seam: `webui/src/lib/gatewayDiff.ts` 纯函数

输入：current/proposed JSON

输出：
- `diffGateway` → `{ added, removed, changed }` 按顶层与 models 数组分段
- `detectConflicts` → 冲突键列表
- `validateGatewayJson(text) -> { ok, error, value }` 校验 JSON 合法性与最小 schema

验收：
- 非法 JSON 返回 error 含行列提示
- 缺少 api/baseUrl/models 时校验失败
