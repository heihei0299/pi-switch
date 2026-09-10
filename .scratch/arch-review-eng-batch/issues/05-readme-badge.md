# 05: README 徽章与 package.json 版本一致（A14）

**What to build:** 两个 README 的版本徽章不再落后于 `package.json`。

**Blocked by:** None — can start immediately

**Status:** resolved (2026-09-11)

- [x] `README.md` 与 `README_ZH.md` 的版本徽章更新为 `package.json` 的当前版本
- [x] 不改徽章形态（仍为静态 badge），不引入动态 release badge
- [x] 校验两者一致

## 实施记录

- 两个 README 第 5 行的 `version-20260902.0.0-blue.svg` 改为 `version-20260910.0.2-blue.svg`，与 `package.json` 的 `20260910.0.2` 一致
- 未改为动态 release badge：那会引入新的外部依赖与渲染时序问题，超出本项"消除漂移"的目标

## 验证证据

- `grep -o 'release-[0-9.]*' README.md` 与 `package.json` 的 `version` 字段比对一致（徽章 URL 中的 `version-20260910.0.2`）

## 未纳入本项

- 发布流程中"同步徽章"的自动化（CI 校验徽章与 package.json 一致）可另立项；本项只消除当前漂移。
