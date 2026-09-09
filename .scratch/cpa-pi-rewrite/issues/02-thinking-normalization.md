Labels: wayfinder:research
Type: research
Status: resolved

## Question

CPA 的 `thinking/validate.go:ValidateConfig`（后缀解析→Budget↔Level 互转→`clampLevel/clampBudget`→`allowClampUnsupported` 跨家族降级）的正规化管线，如何映射到 `pi-switch` 的 `thinkingLevelMap` 字符串映射？`CapabilityBudgetOnly/LevelOnly/Hybrid` 三态与 `models.dev` 的 `reasoning` 字段如何衔接，且 `fromSuffix` 的宽松校验在 Rust 侧如何表达？

Blocked by:

## Answer

已落盘 `.scratch/research/02-thinking-normalization.md`（215 words）。要点：CPA 三态 + `levelToBudgetMap`/`Threshold` + `clampLevel` 就近取低 + `strictBudget` 与 `allowClampUnsupported` 跨家族逻辑；pi-switch 现状 `reasoning:bool` + `thinkingLevelMap` 无数值能力。建议新建 `src-rust/thinking.rs` 移植该管线，保留 `thinkingLevelMap` 兼容层。详见研究文档。
