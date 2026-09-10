# 02: 三个子命令共用严格 flag 解析

Status: resolved (2026-09-11)

**What to build:** `duplicate` / `expose` / `fetch-models` 用同一份 flag 扫描：只接受各自允许的 `--flag value`，未知 flag、缺失值、重复 flag、多余位置参数一律拒绝（`provider <sub>: ...` 到 stderr，退出 1）。

**为什么**（评审 Standards + Spec 两轴共同指出）：三个子命令三套宽严不一的循环。
- `duplicate` 末位 `--as` 无值时报 `unknown argument "--as"`（与 add 的「requires a value」不一致）。
- `expose` 把未知 `--flag` 静默当作 model id。
- `expose <name>`（无 id）会**清空** `exposedModels` 却打印 `Exposed 0 model(s)`，是真实的静默破坏。

**Seam**：CLI `handleProvider`（stderr + 退出码 + `provider show`）。

**验收**
- [ ] `fetch-models p --bogus x` → 退出 1，报未知参数，不静默丢弃
- [ ] `duplicate p --as`（无值）→ 退出 1，报「requires a value」
- [ ] `duplicate p --as a --as b` → 退出 1（重复 flag）
- [ ] `expose p --bogus` → 退出 1，`--bogus` 不被当成 model id
- [ ] `expose p`（无 id）→ 退出 1，且 `exposedModels` 未被清空
- [ ] `expose p m1 --channel c` 等既有可用形态不受影响

**测试**：`cmd/pi-switch/provider_flags_test.go`。
