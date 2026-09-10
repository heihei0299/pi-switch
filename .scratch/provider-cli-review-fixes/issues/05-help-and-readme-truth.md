# 05: help 与 README 与现实一致，并由测试固定

Status: resolved (2026-09-11)

**What to build:**
1. 顶层 `Commands:`（`main.go:48`）补齐 `provider` 子命令集合与 `--models`；`provider --help`（`:405`）列出别名与 `--channel` 要求。
2. `README_ZH.md:300` 去掉重复的 `--channel`。
3. `README.md:335` 与中文姊妹行对齐（补 `--channel main`）。
4. 新增测试：help 文本必须覆盖全部实际可用的子命令（含别名），README 中出现的每个 `provider <sub>` 都必须真的可识别。

**为什么**（评审两轴）：以「让文档符合现实」为目的的提交反而让顶层 help 丢了 `duplicate`（`b6a38b2` 是 `add | delete | duplicate --as`，改后是 `add [flags] | use | delete`），且始终不含 `test|fetch-models|expose`；`remove`/`rm`/`ls` 实测可用却两处 help 都不提；`provider --help` 完全没提 `--channel`；`README_ZH.md:300` 出现 `--channel <渠道> --channel main`。这类漂移已反复出现，故改为机械约束（AGENTS.md：能由测试约束的规则不要堆进提示词）。

**Seam**：CLI help 输出 + 仓库 README 文本（测试作为规格）。

**验收**
- [ ] 测试枚举 dispatcher 实际处理的子命令，缺一即失败
- [ ] 测试从两个 README 抽出 `provider <sub>` 并逐个执行，出现「unknown provider subcommand」即失败
- [ ] `README_ZH.md:300` 无重复 flag
- [ ] README.md / README_ZH.md 的 expose 例子形态一致

**测试**：`cmd/pi-switch/provider_help_test.go`。
