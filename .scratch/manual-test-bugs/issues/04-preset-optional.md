# 04 — 新建供应商不填预设报 preset is required

**现象：** WebUI 新建供应商时预设下拉留空（明确写着“预设（预填）”并提供“— 无 —”选项），保存时后端报 400 `preset is required`，自定义上游无法创建。

**复现步骤：**
1. WebUI → Profiles → 添加供应商，预设选“无”，填写 `api`/`baseUrl`/`apiKey`。
2. 保存 → `POST /api/profiles` 返回 400 `{"error":"preset is required"}`。

**预期：** 预设为空时可正常创建（它只是预填模板提示）。

**实际：** 400 拒绝。根因：`handlePostProfile` 强制要求 `preset` 非空，但 WebUI 前端允许并主动省略空预设（`preset: preset || undefined`），前后端打架；且语义上 preset 仅用于预填回填与模型目录推断兜底（`resolveModelsDevProvider` 本就 nil-safe，为空时 enrich 跳过告警），不该是必填项（用户真实上报，手动测试时也曾被迫填 `openai` 占位绕过）。

**Blocked by:** None — can start immediately

**Status:** resolved

- [x] 不填预设建自定义供应商返回 200
- [x] 填预设的老流程不受影响
- [x] 为空时模型目录推断走 `modelsDevProvider`，为空则 enrich 跳过告警、不致命
- [x] `go vet` + 相关测试通过

## 建议修复方向

删掉 `handlePostProfile` 的必填校验（全仓库仅此一处，`PUT` 路径本来就没这个要求）。

## Comments

- 来源：用户真实上报（opencode zen 自定义供应商场景）。
- 前端无需改动：空预设本来就不会发出该字段。

## 实施总结

- 提交：`39dbe2f` — fix: preset optional when creating supplier (manual-test-bugs/04)
- 实现：删除必填校验块，代之以注释说明 preset 语义；`resolveModelsDevProvider` 保持不变。
- 验收标准：隔离环境（`/tmp/pitest4`，43212 端口）实测——不填 preset 创建返回 200，删除 200，daemon 正常回收；`go vet ./internal/server/` 通过。
- typecheck：`go vet ./...` 通过。
- 文档对齐：无文档声称 preset 必填，无需更新。
- 遗留：无。
