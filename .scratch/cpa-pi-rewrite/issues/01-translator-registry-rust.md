Labels: wayfinder:research
Type: research
Status: resolved

## Question

CPA 的 `translator` 自注册（40+ 子包 `init()` 导入即注册）如何在 Rust 侧以 `pi-switch` 的 `proxy.rs` 单文件双路径现状下复刻？`init` 机制、`_ import` 约束（`AGENTS.md: 不单独改 translator`）与 Rust 的 `inventory` / `linkme` / 显式 `register!` 宏，哪一条能保持“无状态纯函数 + 集中路由”且不破坏现有 `Responses Passthrough/Convert` 的 `api` 能力声明？

Blocked by:

## Answer

已落盘 `.scratch/research/01-translator-registry-rust.md` @ `research/01-translator-registry-rust:245c2e3`。结论：显式集中表（`const` 数组）优于 `inventory/linkme`，`pi-switch` 仅 `is_native_responses_passthrough` + `is_chat_completions_convert` 二分支 + `responses_to_chat`/`ChatSseToResponses` 纯函数，无需 linker 魔法；`api×responsesMode` 仍为唯一路由依据。
