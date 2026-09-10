# 04: 消除 ADR 编号重复（A13）

**What to build:** `docs/adr/` 编号唯一且连续，引用 "ADR 0004" 不再有歧义。

**Blocked by:** None — can start immediately

**Status:** resolved (2026-09-11)

- [x] 两份 `0004-*` 之一重编号，编号与引入时间同时单调
- [x] 文件内标题与文件名一致
- [x] `docs/`、README、CONTEXT.md 等活引用同步更新
- [x] `.scratch` 下历史 spec 不回改（历史记录）
- [x] `docs/adr/` 编号唯一且连续

## 实施记录

- 按引入时间定序：`0004-responses-provider-passthrough`（2026-08-07）< `0005-models-dev-as-model-metadata-source`（08-28）< `0004-supplier-side-pi-session-scan`（08-31）
- 故把最后引入的 `0004-supplier-side-pi-session-scan.md` 重编为 **`0012`**（当前最大编号为 0011），文件内标题同步为 `# ADR-0012: …`——这样编号顺序与时间顺序一致，且不改变任何既有决策内容
- 内容零改动，只改编号
- `.scratch` 中指向旧编号的引用（`fix-400-context-overflow/spec.md`、`stats-legacy-compat/spec.md`、`cpa-pi-rewrite/issues/03`）按惯例不回改；其中指向该 ADR 的编号已成为历史记录

## 验证证据

- `ls docs/adr/` → `0004-responses-provider-passthrough`、`0005`、`0006`…`0011`、`0012`，无重复
- `grep -rn "0004-supplier-side" docs/ README.md README_ZH.md CONTEXT.md` → 仅剩 `architecture-review.md` 中标注为"修复前"的历史叙述
