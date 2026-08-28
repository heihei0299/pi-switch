# Spec: 模型目录 enrich（models.dev 元数据更新）

Status: ready-for-agent

## Problem Statement

用户为 provider 配置模型时，本地 `ModelEntry` 的 `cost / contextWindow / maxTokens / reasoning / input / name` 依赖手工填写或上游 `/v1/models` 返回——但上游仅返回 id 列表，无 pricing 与 limit 等元数据，导致消费估算不准、窗口与能力标记缺失。用户期望“每次更新模型时从模型目录（https://models.dev）自动补齐元数据”，但该意图未被产品化：无明确的数据源角色、触发范围、Provider 映射与覆盖策略，手工定制亦可能被静默覆盖。

## Solution

引入模型目录作为模型元数据的权威源（ADR 0005）。所有“发现/更新模型”的链路在获得模型 id 列表后，按 provider 维度到 `https://models.dev/api.json` 目录查询对应元数据，并按分字段策略 enrich 到本地 `ModelEntry`；未在目录中命中的模型保留原值，不自动新增目录独有模型；目录全量缓存于本地并以 24h TTL 刷新，离线或失败时降级到缓存或原值。全程可观测（toast/日志中报告 enrich/跳过/失败数）。

## User Stories

1. 作为用户，我在 TUI/WebUI 点击 Fetch Models 时，系统自动用模型目录 enrich 选中模型的 cost/limit/reasoning/input，使消费与窗口无需手工维护。
2. 作为用户，我为某个 profile 显式配置 `modelsDevProvider`（如 `openai`），使自定义名称的 profile（如 `my-openai`）仍能正确映射到目录的 provider。
3. 作为未配置映射的用户，我依赖 preset 自动推断目录 provider（如 `openrouter→openrouter`），推断失败时 enrich 静默跳过且不阻塞 Fetch。
4. 作为手工定制模型的用户，我已填写的模型 name 不被目录覆盖（仅缺省时补齐），而 cost/limit/reasoning/input 按目录覆盖以保证准确性。
5. 作为拥有私有/自建模型的用户，我的本地模型在目录中找不到时保留原值，不被清空也不报错。
6. 作为离线用户，我在无网络时仍能用 24h 内的本地缓存完成 enrich，无缓存且无网络时保留原值并提示失败原因。
7. 作为频繁 Fetch 的用户，我在 24h 内多次 Fetch 时命中本地缓存（`~/.pi-switch/cache/models-dev.json`），无需每次拉 4.4MB 全量。
8. 作为用户，我在 Fetch 成功 toast 中看到“已 enrich N 条 / 跳过 M 条（目录未覆盖）/ 失败原因”，可判断元数据是否已更新。
9. 作为使用 `pi-switch provider models <name> ...` 手工指定模型的用户，我的显式列表不受目录约束（不要求 id 必须在目录中）。
10. 作为新增 provider 的用户，我在初始模型发现阶段同样获得目录 enrich，而非仅在后续 Fetch 时。
11. 作为目录新增模型的用户，我不会被静默自动新增模型；需在 Fetch 勾选后才会加入本地列表。
12. 作为开发者，我可通过测试验证 enrich 的字段映射、分字段覆盖、未命中保留、TTL 与降级行为。

## Implementation Decisions

- 模块与 seams：主 seam 为 `ops` 层的 enrich 链（`src-rust/ops.rs` 的 `fetch_models` / `update_provider_models` 路径内新增 `enrich_models_from_catalog`），伴随 `config::ProviderProfile` 新增可选字段 `modelsDevProvider: Option<String>`（`src-rust/config.rs` 与 `webui/src/types.ts` 同步，`Some` 时显式映射，`None` 时按 preset→目录 key 推断）。目录获取封装为 `catalog` 模块（`src-rust/catalog.rs`，职责：`get_or_refresh_catalog()` 负责 `https://models.dev/api.json` 拉取、`~/.pi-switch/cache/models-dev.json` 原子写、24h TTL 判定与降级），仍归属主 seam 的 blast radius，不单独暴露网络 service 缝。
- 触发范围：所有“拉取/发现”类路径走目录 enrich（TUI/WebUI 的 Fetch Models、新增/编辑 profile 的自动发现），手工 `provider models` 命令不拦截；`sync_gateway_to_pi` 不直接 enrich，网关模型的元数据随 profile enrich 后自然同步。
- Provider 映射：`modelsDevProvider` 优先；未填时按 preset 推断（`openrouter→openrouter`、`anthropic→anthropic`、`deepseek→deepseek`、`openai→openai`、`siliconflow→siliconflow` 等），推断失败则跳过 enrich，不报错。
- 字段映射与覆盖（Q4 分字段策略）：`limit.context→contextWindow`、`limit.output→maxTokens`、`cost.input/output/cache_read→cost.input/output/cacheRead`（单位 $/1M 直接透传，含 `tiers/context_over_200k` 时按最外层 cost 映射）、`reasoning→reasoning`、`modalities.input→input`、`name→name（仅缺省时补齐，手工非空保留）`；其余 `extra/compat/headers/thinkingLevelMap` 不由目录覆盖。
- 列表语义：仅 enrich 已有 `profile.models` 中已存在的 id；目录有但本地无的模型不自动新增，需用户在 Fetch 勾选后经 `update_provider_models` 新增并在下次 enrich 时补齐元数据。
- 未命中策略：目录无该 id 时保留本地原值，不清 cost/limit；命中但字段缺失则保留原字段。
- 目录获取与缓存：`GET https://models.dev/api.json`（超时 10s，与现有 provider 拉取一致），成功后原子写入 `~/.pi-switch/cache/models-dev.json`；TTL 24h，命中期直接读缓存；过期或无缓存时尝试网络，失败则回退到过期缓存（若有）并报告 warning，无缓存且失败则跳过 enrich。目录 payload 约 4.4MB，全量缓存不做切片。
- 可观测性：enrich 结果以 `enriched=N skipped=M failed=K` 形式合并到现有 Fetch 成功 toast 与 `web.rs` 响应中；`skipped` 为目录未覆盖数，`failed` 为网络/解析失败数，失败原因写入日志。
- 校验：`validate_provider_profile` 中若 `modelsDevProvider` 非空，校验其为目录已知 provider key 的子集时仅作 warning（不阻断，避免目录新增 provider 时的硬编码滞后）；未知 key 时跳过 enrich。
- 尊重 ADR 0005 与 CONTEXT.md 术语：spec 与代码注释、日志文案统一使用“模型目录 / 模型元数据”，与“上游模型列表”区分。

## Testing Decisions

- 仅测外部行为，不测实现细节；所有测试位于主 seam `ops` 层（Rust `cargo test`），无需为 `web.rs`/TUI 单独补 UI 测试。
- 覆盖点：enrich 字段映射正确性（context/output/cost/reasoning/input/name 分字段策略）、未命中保留原值、目录新增模型不自动落地、TTL 命中与过期行为、网络失败降级到缓存、无缓存失败跳过、preset 推断与显式映射优先级、手工 `provider models` 不受目录约束。
- 先行参考：现有 `config::tests` 的 profile 校验模式与 `ops::parse_model_ids` 的 payload 解析测试；新增 catalog/enrich 测试沿用同类纯函数 + 临时文件隔离方式，不依赖真实网络（以 fixture `api.json` 片段注入）。
- WebUI 仅需补充 `webui/src/types.ts` 的类型回归（`modelsDevProvider` 可选字段），不新增 `vitest` 用例。

## Out of Scope

- 不将模型目录作为模型“存在性”权威（不以目录 id 列表替代上游可用性判断）。
- 不提供目录的增量同步、按 provider 切片拉取或版本化 diff（始终全量 api.json）。
- 不自动定时后台刷新（仅在 Fetch/发现时按需触发）。
- 不接管非网关 provider 的 `models.json` 自由编辑。
- 不提供 WebUI 的目录 provider 手动搜索/浏览页（映射字段为输入框，校验提示即可）。

## Further Notes

- `models.dev` 的 `cost.tiers / context_over_200k` 为目录特有形态，本期仅映射最外层 `cost`，tiers 保留原值，不做阶梯 pricing 展开。
- 目录已知 provider key 随上游目录演进而变化，代码中不硬编码白名单，校验仅作提示。
- 缓存路径 `~/.pi-switch/cache/models-dev.json` 与现有 `~/.pi-switch/backups/` 同级，复用 `config::config_dir()` 推导。
