Status: ready-for-agent
Type: task

# 06 thinking.rs 正规化管线

## 需求

依据 `spec.md §4.1` + `ADR-0007`，新建 `src-rust/thinking.rs`，移植 CPA `thinking/validate.go` 正规化管线：

- 解析 `ThinkingConfig{mode, level, budget}` 与 `ModelInfo{ThinkingSupport{levels, min, max}}`
- 三态 `CapabilityBudgetOnly/LevelOnly/Hybrid` 判定
- Budget↔Level 互转（`levelToBudgetMap`）
- `clampLevel`（就近，tie 取低）/ `clampBudget` 到模型区间
- `allowClampUnsupported` 跨家族降级（`max→high`）与 `fromSuffix` 宽松校验
- 保留 `thinkingLevelMap` 兼容回退，不阻断请求
- 新增 `ProxySettings.safety_tokens: Option<u64>`（默认 16384）

## Seams（测试接缝）

- `normalize_thinking(model, input, fromFmt, toFmt, fromSuffix) -> Result<ThinkingConfig>`
- `clamp_level(level, modelInfo, toFmt) -> level`
- `clamp_budget(budget, modelInfo, toFmt) -> budget`

## 验收

- 三态转换、clamp、跨家族降级均有单测覆盖（复用 CPA `thinking_conversion_test.go:2545` 用例）
- 失败回退 path 不抛错
- `cargo test 355+` 不破

Blocked by: 02, 04
