# 02: `provider duplicate` 接线

**What to build:** `pi-switch provider duplicate <name> --as <new>` 真实复制 profile。

**Blocked by:** 01

**Status:** ready-for-agent

- [ ] 抽出 `handleDuplicateProfile` 的核心为可共用函数，handler 改为委托
- [ ] CLI 接线；`--as` 必填，缺失/目标已存在/源不存在时 stderr + 非零
- [ ] 测试：duplicate 后 list 含新名字且内容与源一致；失败路径断言
