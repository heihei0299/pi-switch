# 04: 消除 ADR 编号重复（A13）

**What to build:** `docs/adr/` 编号唯一且连续，引用 "ADR 0004" 不再有歧义。

**Blocked by:** None — can start immediately

**Status:** resolved (2026-09-11)

- [x] 两份 `0004-*` 之一重编号为最大编号 +1（`0012`），不声称全局时间单调（见实施记录）
- [x] 文件内标题与文件名一致
- [x] `docs/`、README、CONTEXT.md 等活引用同步更新
- [x] `.scratch` 下历史 spec 不回改（历史记录）
- [x] `docs/adr/` 编号唯一且连续

## 实施记录

- 两份 `0004` 中保留**先引入**的 `0004-responses-provider-passthrough`（2026-08-07），把后引入的 `0004-supplier-side-pi-session-scan`（原始引入 2026-08-31，commit `4686b45`）重编为 **`0012`**（当前最大编号 0011 + 1，符合 ADR-FORMAT.md 的规则），文件内标题同步为 `# ADR-0012: …`，内容零改动
- **边界说明**：这只保证编号唯一且"新编号最大"，**不保证编号与时间全局单调**——该 ADR 在时间上早于 0007–0011（09-02…09-08），真正的全局时间重排需要改动 5 个文件及其引用，代价与收益不成比例。首版实施记录曾写成"编号与时间同时单调"，属表述过头，经 review 指出后已改正
- 内容零改动，只改编号
- `.scratch` 中指向旧编号的引用按惯例不回改（历史记录）。**经 review 更正清单**：真正指向本 ADR 的是 `.scratch/pi-session-supplier-scan/issues/01-04`（"`docs/adr/0004` 已描述"）与 `.scratch/remove-rust-core/spec.md:60`；首版记录里举的 `fix-400-context-overflow/spec.md:67` 指的是**另一个** 0004（Responses 透传），它未被改名、也不算 stale。另有 review 指出的 `stats-legacy-compat/spec.md:48`、`cpa-pi-rewrite/issues/03` 同样指向旧编号语义

## 验证证据

- `ls docs/adr/` → `0004-responses-provider-passthrough`、`0005`、`0006`…`0011`、`0012`，无重复
- `grep -rn "0004-supplier-side" docs/ README.md README_ZH.md CONTEXT.md` → 仅剩 `architecture-review.md` 中标注为"修复前"的历史叙述
