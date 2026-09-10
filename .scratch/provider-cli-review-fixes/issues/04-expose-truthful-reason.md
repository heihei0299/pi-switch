# 04: `provider expose` 零渠道给出真实原因

Status: resolved (2026-09-11)

**What to build:** `provider expose` 在 profile 渠道数不为 1 且未给 `--channel` 时，按真实情况分别报错：0 个渠道说「没有渠道」（并指出需要先有带 baseUrl 的渠道），多个渠道才说「有多个渠道」。

**为什么**（评审 Spec 轴）：`main.go:528` 只在**恰好 1 个**渠道时推断渠道名，0 渠道时落到 `:532` 那条固定文案 `--channel <channel> required (profile has multiple channels)`。实测旧形状 profile（只有 `{"baseUrl":…}`、无 `upstreams`）会打印「有多个渠道」，而它一个渠道都没有。spec 判据要求 stderr 给出**真实**原因。

**Seam**：CLI `provider expose` 的 stderr + 退出码。

**验收**
- [ ] 0 渠道 profile → 退出 1，说明「没有渠道」，且**不**声称「multiple channels」
- [ ] 多渠道 profile → 仍要求 `--channel`，文案不变
- [ ] 单渠道 profile → 仍自动推断（既有测试保持绿）

**测试**：`cmd/pi-switch/provider_flags_test.go`（0 渠道用例）。
