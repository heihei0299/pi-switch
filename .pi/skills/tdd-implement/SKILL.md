---
name: tdd-implement
description: "TDD seam red-green loop: use when the user provides a spec/ticket to implement test-first, or mentions TDD/red-green/test-first and wants the full loop through typecheck, review, commit and closeout. For implementation without TDD use implement; for TDD technique alone use tdd."
---

# TDD Implement

`seam` + `red-green` 为领衔词的完整实现编排：每个 seam 一个红-绿循环，直到 commit。TDD 语义（红-绿循环、seam 定义、好测试标准）以 [tdd 技能](.agents/skills/tdd/SKILL.md) 为唯一事实源——测试标准见 [tdd/tests.md](.agents/skills/tdd/tests.md)，Mock 边界见 [tdd/mocking.md](.agents/skills/tdd/mocking.md)；本技能只编排阶段与运行时规则。

本技能是**长程任务**（Long-Horizon Skill）：多阶段串行执行，自带**回合连续性**（Turn Continuity）与**任务分解**（Chunking）规则。术语定义见 `CONTEXT.md`，技能设计规则见 `docs/agents/skill-design.md`。

## 分支

- **单线**：单 spec / 单 issue，走下节 Steps ①→⑦（详规见 [stages.md](references/stages.md)）。
- **多 issue 编排**：`.scratch/<feature>/issues/` 下多文件且含 `Blocked by` 时走编排模式——编排器按依赖分层并行调度，子代理各自治完成 ①→⑦。详见 [orchestration.md](references/orchestration.md)。

## Steps

按序执行，每步达到完成条件才进入下一步；进入任一步前先读取其在 [stages.md](references/stages.md) 的定义。

| Step | 做什么 | 完成条件（可验证） | 详规 |
|------|--------|-------------------|------|
| ① 理解需求 | 读取 spec/ticket + `CONTEXT.md`/`docs/adr/`，澄清歧义 | 能复述需求且无未澄清歧义 | [stages.md#阶段-①](references/stages.md#阶段-①理解需求) |
| ② 确认 Seams | 列出待测公共接口 seams（名称+输入+预期输出），向用户确认并生成 Todo | 用户明确同意 seams 清单；Todo 已生成 | [stages.md#阶段-②](references/stages.md#阶段-②确认-seams测试接缝) |
| ③ TDD 开发循环 | 逐 seam 红-绿循环，串行推进至全绿 | 所有 seams 红-绿完成 + typecheck 通过 | [stages.md#阶段-③](references/stages.md#阶段-③tdd-开发循环) |
| ④ 完整测试套件 | 跑全量测试 | 全部测试通过（失败回 ③） | [stages.md#阶段-④](references/stages.md#阶段-④完整测试套件) |
| ⑤ Code Review | 按 [code-review](.agents/skills/code-review/SKILL.md) 双轴审查（Standards + Spec） | 双轴均通过 | [stages.md#阶段-⑤](references/stages.md#阶段-⑤code-review) |
| ⑥ Commit | 跑 [commit-check](.agents/skills/commit-check/SKILL.md) 门禁四项后提交 | commit 完成且历史校验通过 | [stages.md#阶段-⑥](references/stages.md#阶段-⑥commit) |
| ⑦ 收尾 | 文档对齐 → issue 状态与实施总结 → 目录卫生 | 文档已对齐、issue 已 `resolved`+总结落盘、工作区干净 | [stages.md#阶段-⑦](references/stages.md#阶段-⑦收尾文档对齐--issue-状态--实施总结) |

编排模式流转见 [orchestration.md](references/orchestration.md)；子代理内部仍走上表 ①→⑦（其中 ④ 为相关测试口径，全量由编排器收敛）。

### 阶段间流转

- 正常流转：出口条件满足即进入下一阶段，不在阶段间停顿。
- 回退路由：见 [stages.md#回退路由](references/stages.md#回退路由)；编排模式回退见 [orchestration.md#A5](references/orchestration.md#a5-回退与冲突)。
- 回合连续性：阶段内连续动作（红→绿→typecheck→下一 seam）在**一个回合内串行完成**，直至阶段出口；预告下一步后立即执行。详规见 [stages.md ③-3e](references/stages.md#3e-回合连续性) 与 [orchestration.md#A2](references/orchestration.md#a2-分层调度)。
- 任务分解：巨型写入拆小步——`write` 超 ~150 行先写骨架再分批补全，`replace` 超 ~5 处分批执行并验证。详见 [stages.md ③-3f](references/stages.md#3f-任务分解chunking)。

## 引用

- TDD 核心规则：[tdd 技能](.agents/skills/tdd/SKILL.md)
- 测试标准：[tdd/tests.md](.agents/skills/tdd/tests.md)
- Mock 指南：[tdd/mocking.md](.agents/skills/tdd/mocking.md)
- Commit 门禁：[commit-check](.agents/skills/commit-check/SKILL.md)
- 单线详规：[stages.md](references/stages.md)
- 多 issue 编排：[orchestration.md](references/orchestration.md)
- Issue tracker 约定：[issue-tracker.md](../../docs/agents/issue-tracker.md)
