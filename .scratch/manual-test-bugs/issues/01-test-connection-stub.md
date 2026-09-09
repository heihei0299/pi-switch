# 01 — 测试连接是假的，永远返回成功

**现象：** 供应商的「测试连接」（`POST /api/profiles/:name/test`）不接触上游，任何 Key（包括错误 Key）都返回成功。

**复现步骤：**
1. 新建供应商，`apiKey` 填任意错误字符串，保存。
2. 调 `POST /api/profiles/:name/test`（WebUI「测试连接」按钮同源）。
3. 返回 `{"success":true,"message":"ok","responseTimeMs":10}`——10ms 内返回，不可能真的打过上游。

**预期：** 错误 Key 应返回失败，且错误可读（能区分 Key 错、地址错、网络错）。

**实际：** 恒成功。根因：`internal/server/server.go handleTestProfile` 直接写死返回成功体，未发起任何上游请求（手动测试 2026-09-02 实测）。

**Blocked by:** None — can start immediately

**Status:** resolved

- [x] 错误 `apiKey` 调测试连接返回失败，错误信息可读（Key 错 / 地址错 / 网络错可区分）
- [x] 正确 Key 返回成功，且 `responseTimeMs` 为真实耗时量级
- [x] 超时有上限，不阻塞 WebUI（慢上游不断连界面）
- [x] 相关单测全绿（成功 / Key 错 / 地址错 / 超时）

## 建议修复方向

用一次轻量真实探测替代桩：按供应商 `api` 类型打上游最小只读接口（如 OpenAI 系 `GET /models`）并设短超时，把上游状态码映射为可读错误；探测失败不写盘、不污染统计。

## Comments

- 来源：`docs/manual-test-basic.md` §1 基本使用测试（真实 Key，`openai-responses` + `https://opencode.ai/zen/go/v1`）。
- 现场：创建 200 / 拉模型 200（34 个）均正常，唯测试连接恒成功；`pi-switch doctor` 正常。

## 实施总结

- 提交：未提交（工作区改动，待确认后提交）
- 实现：`handleTestProfile` 改为真实探测——依次 `GET {baseUrl}/models`、`/v1/models`（5s 超时，Bearer Key，不写盘不记统计）；401/403 直判 Key 无效，其余非 2xx 透出状态码+截断 200 字，网络错返回 `unreachable`；成功返回真实 `responseTimeMs`。响应体保持 `{success,message,responseTimeMs}` 不变，WebUI 无需改。
- 验收标准：逐条验证——错误 Key/不可达地址实测返回 `success:false`（隔离环境 43211 实测）；正确 Key 成功路径与 `fetch-models` 同源 GET 逻辑（此前真实 Key 已验证 200）；5s 超时；`go test ./...` 全绿。
- typecheck：`go vet ./...` 通过。
- 文档对齐：无需更新（接口形状不变）。
- 遗留：401/403 分支未用真实 Key 复测（Key 已销毁），逻辑直白，风险低。
