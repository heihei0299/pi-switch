---
name: commit-check
description: "Run the pre-commit gate before any commit: verify docs match the implementation, align README, keep the directory clean, and write a clear commit message. Use whenever the user is about to commit or asks to check anything about the commit — e.g. verifying docs/README are in sync, cleaning up temp files, scanning for secrets/keys/.env in the change, or having you write the commit message. Not for general PR/code review (that's code-review), and not for explaining git/commit conventions (that's a teach task)."
---

# Commit Check

提交前的**门禁检查**：审查文档 → 对齐 README → 保持目录卫生 → 规范 commit message，四项全过才允许 commit。本技能是轻量检查清单，不重写 code-review 的审查语义（[code-review](.agents/skills/code-review/SKILL.md) 是唯一事实源），也不替代任何完整实现流程——它是任何 commit 前的通用门禁，无论改动来自哪个流程。

## 四项检查（全部通过才 commit）

### ① 审查文档

- 本次改动涉及的行为/接口/配置/命令是否有对应文档（README、`docs/`、技能正文）描述
- 文档描述与实现一致：无过期信息、无声称未实现的功能、无遗留的旧接口描述
- 涉及技能/模板/配置改动时，检查正文引用的路径与实际一致（如相对路径、目录结构）
- 发现不一致 → 先修文档（或更新实现），再进入下一步

### ② 对齐 README

- 改动涉及项目结构、分发文件、技能/命令清单时，检查 README 中对应的结构说明、映射表、清单是否同步
- 改动涉及用法/CLI/配置/示例时，检查 README 对应描述与实际一致
- 存在模板镜像/分发副本时，确认源文件与副本同步（如有守护测试，跑一遍确认）
- **特例：skill 与 AGENTS.md 的模板同步增量**：若 `git diff HEAD` 仅涉及 `AGENTS.md` 的 `tdd-implement ↔ implement` 路由行 + `.agents/skills/`/`.pi/skills/`/`.opencode/skills/` 的技能文件 + `.gitignore` 的 `.pi/` 忽略，且同目录存在 `AGENTS.md.bak`（模板同步 `sync`/`init --force` 的备份），视为模板同步的预期增量，禁止自动 `checkout -- AGENTS.md` 回滚；存在性按文件系统判（`ls .agents/skills/<name> .pi/skills/<name> .opencode/skills/<name>` 任一存在即算存在，不以 `git ls-files` 为准）；缺技能则正向补齐而非回滚文档

### ③ 保持目录卫生

- `git status` 确认工作区只含预期改动：无残留未跟踪文件、无临时产物（调试脚本、日志、备份文件、`[DEBUG-...]` 残留）
- 清理本次改动产生的临时文件（一次性脚本、转储、探针）——仅删本次产生的未跟踪临时产物，禁止为达干净而执行 `git reset --hard`、`git checkout .`、`git clean -fd`、`git stash push --include-untracked`、`git push --force`、`git rebase -i` 等（需显式用户确认；`stash` 如需使用改用 `--keep-index` 并在 `pop` 后校验 `git merge-base --is-ancestor $BASE_HEAD HEAD`）。详见 `CONTEXT.md` Git History Preservation 与 `docs/agents/skill-design.md` Rule 4
- 若本次会话记录了 `BASE_HEAD`，commit 前校验 `git merge-base --is-ancestor $BASE_HEAD HEAD`，失败即经 `git reflog` 恢复后才提交
- 确认没有敏感信息进入改动（密钥、token、`.env`、私钥）——跑 `scripts/scan-sensitive.sh`，不用手写扫描
- 提交后工作区应为干净状态（`git status` 无输出）

### ④ 规范 commit message

- 格式遵循仓库约定（常见：`<type>(<scope>): <subject>`，type 用 feat/fix/docs/chore/refactor/test）
- subject 描述变更内容而非过程（不说"我做了什么"，说"改成了什么"）
- 需要时补充 body：动机、影响范围、验收证据（测试结果、同步确认）
- 一次 commit 只含一个逻辑变更；多主题拆多个 commit

## 不做什么

- 不做全量 code review：审查语义以 [code-review](.agents/skills/code-review/SKILL.md) 为唯一事实源，本技能不重写
- 不替代实现流程的收尾：`tdd-implement` 阶段⑦已含文档对齐与目录卫生，本技能只管独立 commit 的门禁
- 不顺手重构：只检查与本次改动直接相关的内容，不扩权到无关文档/目录
- 不发明扫描规则：敏感信息检测跑 `scripts/scan-sensitive.sh`，不每次重写 grep 模式

## 执行顺序（回合内串行）

1. 跑 ① 审查文档 → ② 对齐 README → ③ 保持目录卫生 → ④ 写 commit message
2. 任一项发现问题：修复后重跑该项，全部通过才 commit
3. commit 后确认 `git status` 干净，工作结束

**回合连续性**：四项检查在一个回合内串行完成，不等用户"继续"；发现问题立即修复并重查，直到四项全过或遇到外部阻塞（权限/授权缺失）。

## 出口条件

- [ ] 文档审查通过（无过期/不一致描述）
- [ ] README 对齐（涉及结构/分发改动时已同步）
- [ ] 目录卫生（`git status` 干净，无临时产物/敏感信息）
- [ ] commit message 规范（遵循仓库格式）
- 四项全过 → commit

## 引用

- 代码审查语义：[code-review](.agents/skills/code-review/SKILL.md)（唯一事实源，本技能不重写）
- 完整实现流程：[tdd-implement](.agents/skills/tdd-implement/SKILL.md)（含流程内收尾的文档对齐与目录卫生）
