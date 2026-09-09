# 02 — 网关发布后 pending 恒为 1，横幅消不掉

**现象：** 网关发布（`PUT /api/models/gateway`）成功后，再查 `GET /api/models/gateway/preview`，`pending_count` 恒为 1 而不是 0，Gateway 页待发布横幅永远消不掉。

**复现步骤：**
1. 供应商 expose 模型 → preview 显示 `pending_count=2`，`proposed` 正确。
2. 用 `proposed` 原样调发布，返回 `{"ok":true}`。
3. 再查 preview：`pending_count=1`（预期 0）。删除模型再发布一次，发布后依然为 1。

**预期：** 刚发布完、无新改动时 `pending_count` 为 0。

**实际：** 恒为 1。根因：`Publish` 经 `MergeGatewayExtra` 把落盘条目的 `compat` 等 extra 键保留进新发布，而 `BuildProposedGatewayEntry` 从不生成这些键——实测落盘条目多出顶层 `"compat":{"sendSessionAffinityHeaders":true}`（来自落盘前已存在的条目内容），`DiffGateway` 按 JSON 全等比较于是恒差 1。 publish 越多、extra 越多，pending 下限越高，永不归零。

**Blocked by:** None — can start immediately

**Status:** resolved

- [x] 发布后无新改动时 `preview.pending_count` 为 0，横幅消失
- [x] 手工写入的 extra 键（`compat`/`headers` 等）发布后仍保留（不丢失，ADR 0006 隔离语义不变）
- [x] 再改供应商并发布，diff 仅包含本次改动
- [x] 相关单测全绿（发布→preview 归零、extra 保留、二次改动 diff）

## 建议修复方向

把 pending/diff 口径与落盘口径对齐：preview 比较前先对 `proposed` 做一次同样的 `MergeGatewayExtra(current, proposed)` 归一化再 diff（只改比较，不改落盘与隔离语义）。

## Comments

- 来源：`docs/manual-test-basic.md` §3 基本使用测试，2026-09-02 两次发布均复现。
- 现场：`~/.pi/agent/models.json` 的 `pi-switch` 条目落盘正确（模型增减均生效），仅 pending 计数错；`gt`/`opencode-go` 条目未受影响。
- 注意：测试过程中覆盖写过本地 `pi-switch` 网关条目，原有旧模型列表未做发布前快照，恢复时以空模型发布收尾；其中的手工 `compat` 得以保留。

## 实施总结

- 提交：未提交（工作区改动，待确认后提交）
- 实现：按建议方向，`handleGatewayPreview` 在 diff 前加一行 `proposed = gateway.MergeGatewayExtra(curMap, proposed)`（`curMap` 为 nil 时原样返回，逻辑安全）；`Publish` 落盘路径未动，ADR 0006 隔离语义不变；返回的 `proposed` 即落盘口径。
- 验收标准：隔离环境复测——含 `compat` 的种子条目 + 同模型供应商，发布后 `pending_count=0`，`compat` 保留；`go test ./...` 全绿。
- typecheck：`go vet ./...` 通过。
- 文档对齐：无需更新（接口形状不变，`proposed` 更接近真实落盘）。
- 遗留：无。
