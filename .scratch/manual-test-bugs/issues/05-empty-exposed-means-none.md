# 05 — 拉模型后网关全有、供应商一侧全未暴露，前后不一致

**现象：** 新供应商第一次拉取模型保存后：供应商配置里有全部模型但一个都没暴露，而网关（preview/发布/代理）里却有全部模型。界面显示与实际提供对不上。

**复现步骤：**
1. 新建供应商 → 拉取模型 → 保存（不勾暴露）。
2. 供应商一侧：`exposedModels` 为空，界面显示未暴露。
3. 网关 preview/发布/`GET /v1/models`/代理路由：全部模型都在（空暴露回退到全部）。

**预期：** 新模型默认不暴露，且网关以暴露状态为准——两边一致。

**实际：** 四处“空 exposed = 全部”回退（网关构建、`GET /v1/models`、路由 `exposes()`、创建时默认全选）+ 前端拉取/手填自动勾暴露，导致未暴露的模型被实际提供（用户真实上报）。

**Blocked by:** None — can start immediately

**Status:** resolved

- [x] 拉取保存后不勾暴露：网关 preview 为空，代理不可达
- [x] 显式 expose 后：网关 preview 精确包含，代理可达
- [x] 创建时不再默认全选暴露
- [x] `go vet` + server 全包 + 其余 Go 包 + webui 219 测试全绿

## 建议修复方向

以后端“空 exposed = 不暴露”为准，删四处回退；前端删两处自动勾选（拉取、手填 ID）。

## Comments

- 来源：用户真实上报（新供应商首次拉模型场景）。
- 语义经用户确认：新模型默认不暴露（而非反向的“空即全暴露”）。
- 老数据影响：历史遗留的“有模型、无暴露集”供应商升级后将不再被提供，需手动 expose 一次。

## 实施总结

- 提交：`3bdb45c` — fix: empty exposed means not exposed (manual-test-bugs/05)
- 实现：`gateway.go` 构建跳过无暴露供应商；`handleModels`/`exposes()` 去回退；`POST` 创建去默认全选；`ProfilesPanel` 去两处自动勾选；`gateway_supplier_test.go` 按新语义更新（S5 fixture 补暴露集，S7 断言空暴露）。
- 验收标准：隔离端到端（未暴露 preview 空 → expose 后精确）；`go vet` 过；server 全包（211s）+ 其余 Go 包 + webui 219 全绿。
- typecheck：`tsc --noEmit` 通过。
- 文档对齐：`docs/manual-test-basic.md` §3 补新语义说明；无其他过期描述。
- 遗留：无。
