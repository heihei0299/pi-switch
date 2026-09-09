# 01 - 目录去歧义与网关拉取自动注入

Status: resolved
Scope: `internal/catalog` / `internal/server` 网关与拉取
Spec: `.scratch/auto-enrich/spec.md`

## 背景
- `deepseek` 渠道模型 `deepseek-v4-flash` 在目录中多家重名，`byBare` 判歧义丢弃，`Lookup` 必 `not found`；网关 `FillModels` 只补缺失不覆盖 `128000`，拉取侧 `modelsDevCatalog` 硬编码无 `deepseek`，致 `enriched 0, catalog not found` 需手填。

## 交付
- `internal/catalog/catalog.go`：`Snapshot` 增 `byProviderBare`，`ParseSnapshot` 双写，`LookupWithProvider(id, provider)` 优先 `provider/bare` 再回退 `bare` 唯一，`Lookup` 保持兼容
- `internal/server/server.go`：`enrichProposedModels` 对 `contextWindow/maxTokens/cost` 改为“目录有值即覆盖”（或现有为默认值 `128000/16384` 时覆盖）；`enrichModelsWithCatalog` 改走快照 `LookupWithProvider` 而非硬编码 `modelsDevCatalog`，`deepseek` 拉取即 `enriched 3`
- 单测：`deepseek-v4-flash` 多家重名时按 `provider` 命中 `1048576`，网关 `128000→1048576` 覆盖，拉取 `deepseek` `enriched 3`

## 验收
- `GET /api/models/gateway/preview` 中 `deepseek/deepseek-v4-flash` 为 `1048576/384000` 且 `pending 0`
- `POST /api/profiles/deepseek/fetch-models` 返回 `enriched 3` 且 `config.json` 落盘为 `1048576/384000`
- `go test ./internal/catalog -run TestLookup -count=1` 与 `go test ./internal/server -run TestEnrich -count=1` 通过

## 不做
- 不引入 `tiktoken`；不做熔断；不改 `TUI`
## 实施总结
- 提交：`46cd30b` — `fix(catalog,server): provider-aware auto-enrich with overwrite (01)`
- 实现的 seams：S1 目录去歧义（byProviderBare + LookupWithProvider）、S2 网关拉取覆盖（FillOverwrite + enrichProposedModels）
- 验收标准：
  - [x] deepseek-v4-flash 歧义时按 provider 命中 1048576
  - [x] 网关 128000→1048576 覆盖
  - [x] 拉取 deepseek enriched 3 且落盘 1048576
  - [x] go test ./internal/catalog -run TestLookup 通过
  - [x] go test ./... 通过
- 测试结果：catalog 2 passed, server 3 passed, 全量 9 passed
- typecheck：go vet 通过
- 文档对齐：CONTEXT.md 模型目录定义保持
- 遗留：无
