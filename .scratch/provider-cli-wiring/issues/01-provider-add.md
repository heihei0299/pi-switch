# 01: `provider add` 接线

**What to build:** `pi-switch provider add <name> [--preset <id>] [--api-key <key>] [--base-url <url>] [--api <kind>]` 真实创建 profile（走服务端同一套校验与落盘），并把 `expose` 从 README 移除或接线（本票先移除，见 05）。

**Blocked by:** None

**Status:** resolved (2026-09-11)

- [x] 抽出 `handlePostProfile` 的核心（校验链 + 重名检查 + 落盘）为可共用函数，handler 改为委托
- [x] CLI `provider add` 调用同一函数；成功时打印真实结果，失败时 stderr + 非零
- [x] 重名、非法 responsesMode、非法 api 等失败路径均有非零退出且 stdout 无成功文案
- [x] 测试：add 后 `provider list` 能看到该 profile（真实副作用）；失败路径断言
- [x] `provider --help` 不再列出未实现的子命令
