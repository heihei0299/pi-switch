# Spec: 模型元数据自动注入（去歧义与覆盖）

Status: ready-for-agent
Version: 1.0
Author: pi-switch maintainers
Date: 2026-09-07
Scope: 目录快照 `internal/catalog`、拉取 `handleFetchModels`、网关 `enrichProposedModels`（不含 TUI）

## Problem Statement

用户经 `deepseek` 渠道添加模型后，网关 `pi-switch/deepseek/*` 的 `contextWindow/maxTokens/cost` 仍为添加时的 `128000/16384` 默认值，未从 `https://models.dev` 自动注入 `1048576/384000` 等真值；`POST /api/profiles/deepseek/fetch-models` 返回 `enriched 0, warning catalog not found for deepseek`，需手动改 `config.json` 并重发布才生效。

## Solution

目录按 `provider/bare` 去歧义并对网关侧允许覆盖：`catalog.Snapshot` 增 `byProviderBare` 索引，`Lookup` 优先 `provider/bare` 再回退 `bare` 唯一；`enrichProposedModels` 对 `contextWindow/maxTokens/cost/input/reasoning` 改为“目录有值即覆盖”（或现有为默认值时覆盖）；拉取侧 `enrichModelsWithCatalog` 改走快照而非硬编码 `modelsDevCatalog`，`deepseek` 经渠道拉取即 `enriched 3`。

## User Stories

1. As a pi 用户，我经 `deepseek` 渠道 `fetch-models` 后，`deepseek-v4-flash` 自动得到 `1048576/384000` 与 `cost`，无需手填
2. As a pi 用户，我在网关 `preview` 中看到 `deepseek/deepseek-v4-flash` 已为 `1048576/384000`，`pending 0` 无需重发布后仍为旧值
3. As a pi 用户，我对 `mimo` 等重名 `bare` 模型，网关仍能按 `supplier` 区分正确注入，不因歧义丢弃
4. As a 开发者，我能在 `catalog.Lookup(provider/bare)` 命中 `tokengo/deepseek-v4-flash` 的 `1048576`，`bare` 歧义时不 `not found`
5. As a 运维，我对存量 `128000` 的网关模型重发布一次后即被目录纠正为 `1048576`

## Implementation Decisions

- **目录**：`catalog.Snapshot` 增 `byProviderBare map[string]Meta`（`provider/bare → Meta`），`ParseSnapshot` 对每个 `provider/models` 条目同时写入 `byProviderBare` 与 `hits[bare]`，`Lookup(id, provider)` 先试 `provider/bare` 再试 `byBare` 唯一
- **网关**：`enrichProposedModels` 的 `catalog.FillModels` 调用改为“有值覆盖”：`fillFloat` 对 `contextWindow/maxTokens` 去掉“已有非零不覆盖”判断，目录非零即写；或判定现有为 `128000/16384` 默认值时覆盖
- **拉取**：`handleFetchModelsForChannel` 的 `enrichModelsWithCatalog` 不再查 `modelsDevCatalog` 硬编码，改 `catalog.Ensure()/Lookup` 快照，`providerKey` 取 `resolveModelsDevProvider` 的 `deepseek` 时命中 `tokengo/deepseek-v4-flash` 的 `provider/bare` 条目
- **兼容**：`Lookup` 签名保持 `Lookup(id string)` 兼容旧调点，新增 `LookupWithProvider(id, provider string)` 供网关/拉取侧使用；旧 `bare` 唯一路径保持

## Testing Decisions

- 只测外部行为，不测内部遍历顺序
- **Seams**：`S1 目录`（`catalog.Lookup` 去歧义）、`S2 网关/拉取`（`POST /api/profiles/:name/fetch-models` 与 `GET /api/models/gateway/preview` 的 `contextWindow`）
- **新增**：`catalog_ambiguous_test.go`（`deepseek-v4-flash` 多家重名时 `LookupWithProvider` 命中 `1048576`）、`gateway_enrich_overwrite_test.go`（`128000` 网关模型被 `1048576` 覆盖）、`fetch_enrich_test.go`（`deepseek` 拉取 `enriched 3`）

## Out of Scope

- 引入 `tiktoken` 或模型专属分词器
- `per-conversation` 熔断
- `TUI` 渠道 `api` 编辑

## Further Notes

- 本 fix 是 `fix-400-context-overflow` 的后续，不引入新 `ADR`，`CONTEXT.md` 的 `模型目录` 定义保持
