# 05: help 与 README 与真实能力一致

**What to build:** `provider --help`、顶层 help 与两个 README 只列真实可用的子命令。

**Blocked by:** 01, 02, 03, 04

**Status:** resolved (2026-09-11)

- [x] `provider --help` 的子命令列表 = 实际可用的集合（含未宣传的 use/remove）
- [x] README 的两处 provider 段落逐行核对：`expose` 行移除或接线后保留
- [x] 机械核验：脚本枚举 help/README 中出现的每个 `provider <sub>` 并真实执行，确认无 `unknown subcommand`
- [x] 测试或校验脚本可作为证据留档
