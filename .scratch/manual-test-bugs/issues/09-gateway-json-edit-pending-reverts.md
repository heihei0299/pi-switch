# 09 — Gateway JSON 编辑后刷新回退，pending_count 不归零

**现象：** 在 Gateway 页面编辑模型 JSON 的显示名或 extra，点击「应用到 Pi」返回 200；刷新页面后当前落盘值仍有编辑，但 canonical proposed 回到供应商配置值，页面继续显示待发布。

**复现步骤：**

1. 在本机隔离 HOME 启动真实 Go WebUI、proxy 和 mock upstream。
2. 准备 `mock/main` 的 `model-a`、`model-b`，并将两个模型暴露到 Gateway 候选列表。
3. 使用真实 `agent-browser` 打开 `http://127.0.0.1:46882`，进入「网关」。
4. 勾选 `mock/main/model-a` 和 `mock/main/model-b`。
5. 在 `gateway json` 中将 `model-b.name` 改为 `Model B Browser Edited`，加入 `extra.e2e="browser"`，点击「格式化」后点击「应用到 Pi」。
6. 刷新 Gateway 页面，再请求 `GET /api/models/gateway/preview`。

**预期：**

- `PUT /api/models/gateway` 返回 200 后，编辑后的显示名和 extra 保留。
- 刷新后 current、proposed 与最后一次保存一致。
- 没有新改动时 `pending_count` 为 0。

**实际：**

- 真实浏览器发送 `PUT /api/models/gateway`，HTTP 200。
- 当前落盘的 `models.json` 保留了 `Model B Browser Edited` 和 `extra.e2e`。
- 刷新后 preview 的 proposed 重新从供应商 config 生成，`model-b` 显示为原始 `Model B`。
- 页面重新显示 `待发布数: 2`，current 与 proposed 不一致。

**初步根因：** Gateway apply 直接写入了手工编辑后的 draft，但刷新 preview 时 proposed 仍从供应商 canonical config 重建，未持久化或合并 Gateway 侧的手工 metadata。因此落盘 current 和刷新后的 proposed 不能保持同一事实来源。

**证据：** 本机 `agent-browser` + 真实 Chrome for Testing `151.0.7922.34`，WebUI `20260908.0.2`，没有使用 `/api/*` route mock。`PUT /api/models/gateway` 返回 200，但刷新后 `pending_count=2`。

**与 02 的区别：** issue 02 针对无手工编辑时 extra 导致 pending 恒差；本问题针对用户编辑 Gateway JSON 后，编辑值未回到刷新后的 canonical proposed。

**Status:** ready-for-agent

- [ ] 明确 Gateway 手工 JSON 编辑的持久化事实来源。
- [ ] apply 后刷新页面仍保留显示名和 extra。
- [ ] current/proposed 比较口径一致，无新改动时 `pending_count=0`。
- [ ] 回归测试覆盖 JSON 编辑、apply、刷新和 preview。

## 建议修复方向

选择并固定一种语义：将 Gateway 手工 metadata 写回 canonical 配置，或在构建 proposed/diff 时按约定合并已发布 Gateway metadata；apply、刷新和 preview 必须使用同一口径，不能只让当前 React state 或 models.json 临时显示编辑值。
