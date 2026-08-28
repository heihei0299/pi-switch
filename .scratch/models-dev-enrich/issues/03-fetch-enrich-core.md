# 03 — Fetch 链路 enrich 核心

**What to build:** 在 Fetch/发现链路中按 provider 维度用模型目录 enrich 本地 `ModelEntry` 的元数据，使 cost/limit/reasoning/input 自动准确，手工定制的 name 保留，私有模型与手工命令不受目录约束。

**Blocked by:** 01, 02

**Status:** done

- [x] 在 `fetch_models` / `update_provider_models` 路径内新增 `enrich_models_from_catalog(profile, ids) -> Vec<ModelEntry>`，复用 `get_or_refresh_catalog()`，触发范围：TUI/WebUI Fetch Models 与新增/编辑 profile 的自动发现；`sync_gateway_to_pi` 间接受益
- [x] 字段映射按分字段策略：`limit.context→contextWindow`、`limit.output→maxTokens`、`cost.input/output/cache_read→cost.input/output/cacheRead`（$/1M 直接透传，仅外层 cost，`tiers/context_over_200k` 不展开）、`reasoning→reasoning`、`modalities.input→input` 按目录覆盖，`name` 仅缺省时补齐
- [x] 列表语义：仅 enrich 已有 `profile.models` 中的 id，目录有但本地无的不自动新增（需勾选后才新增）；`extra/compat/headers/thinkingLevelMap` 不由目录覆盖
- [x] 未命中策略：目录无该 id 或字段缺失时保留本地原值，不清 cost/limit，私有/自建模型无报错
- [x] 手工 `pi-switch provider models <name> ...` 显式列表不受目录约束（不要求 id 在目录中）
- [x] 测试覆盖 enrich 字段映射、分字段覆盖、未命中保留、不自动新增分支

commit: bde27bf
