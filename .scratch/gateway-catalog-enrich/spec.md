# 网关注入 models.dev 元信息补齐

Status: ready-for-agent

## Problem Statement

网关聚合只是原样拷贝供应商池内条目。手工录入或早期拉取的模型缺少 `cost/contextWindow/maxTokens/reasoning/input/name` 等元信息时，注入 `models.json` 的条目同样残缺（如 deepseek 渠道条目仅有 id/name/input/contextWindow，无 cost）。用户期望网关注入时用 models.dev 数据自动补齐缺失元信息。

## Solution

新增 `internal/catalog` 包：拉取 `https://models.dev/api.json`（约 4.5MB）并缓存于 `~/.pi-switch/cache/models-dev.json`（24h TTL，过期在预览/发布时同步刷新，失败用 stale 缓存 + warning）。网关预览与发布在现有聚合之后，对提议条目按“只补缺失”规则填充：去网关前缀取末段 id，在快照内全局后段唯一命中时才补（0 或多命中跳过）。name 为空补、contextWindow/maxTokens 为 0 补、input 为空补、reasoning 为 nil 补、cost 为 nil 整设、cost 子字段为 0 且目录非 0 则补子字段；thinkingLevelMap 与其他 extra 键不动。池内数据不写回。预览响应追加 `enrich` 摘要。

## User Stories

1. As a 网关管理员, I want to 发布时缺失 cost/limit 的模型自动补齐, so that 注入 models.json 的条目元信息完整
2. As a 网关管理员, I want to 预览与发布看到同一份补齐结果, so that 所见即所得
3. As a 手工微调模型的用户, I want to 我填写的值不被目录覆盖, so that 定制不丢失
4. As a 离线用户, I want to 缓存有效期内无网络仍能补齐, so that 发布不阻断
5. As a 网关管理员, I want to 缓存过期且网络失败时发布仍成功并带 warning, so that 目录故障不阻断发布
6. As a 网关管理员, I want to 重名模型（多 lab 同名）不被错配, so that 元信息不张冠李戴
7. As a 开发者, I want to 通过测试验证映射/补缺/唯一性/TTL/降级, so that 回归可护航

## Implementation Decisions

- 新包 `internal/catalog`：快照拉取（超时 15s）+ 原子写缓存 + 24h TTL 判定 + 后段索引 + fill-missing；只依赖标准库。缓存路径 env `PI_SWITCH_CATALOG` 可覆盖（测试隔离），默认 `~/.pi-switch/cache/models-dev.json`
- 字段映射：`limit.context→contextWindow`、`limit.output→maxTokens`、`cost.input/output/cache_read→cost.input/output/cacheRead`（$/1M 直接透传）、`reasoning→reasoning`、`modalities.input→input`、`name→name`
- 匹配：网关 id 去前缀取末段；快照内 `lab/model` 后段全局唯一命中才补
- 接入点：`handleGatewayPreview` 在 Merge 后对 proposed.models 填充并计 enrich 摘要；`handlePutGateway` 与 `handleGatewayPublish` 在 Publish 前对写入 models 同样填充
- 预览响应追加 `enrich: {enriched, skipped, stale, warning}`；其余字段与计数口径不变
- 存量 `modelsDevCatalog` 硬编码表与 Fetch enrich 路径不动

## Testing Decisions

- 只测外部行为：catalog 包单测（fixture 快照：映射、补缺、唯一/歧义/缺失、TTL、失败降级）+ 服务端 HTTP 集成（preview/publish 经 file fixture 端到端，网络用 httptest fixture 注入）
- 先例：`internal/gateway/gateway_test.go`、`internal/server/channel_preview_test.go`（`PI_SWITCH_MODELS`/`PI_SWITCH_CONFIG` 隔离）；webui 侧仅 `tsc --noEmit`
- 不依赖真实网络：目录 payload 以 fixture 注入

## Out of Scope

- 回写渠道池；Fetch 链路改动；硬编码表删除
- thinkingLevelMap 与 extra 键的目录覆盖
- 缓存预热/后台定时刷新；按 provider 切片拉取
- 预览 UI 的 enrich 展示（仅响应字段，后续另起）

## Further Notes

- 约束来源：ADR-0005（模型目录权威源）；CONTEXT.md 术语：模型目录/模型元数据/网关发布
- README-146 的“24h 缓存”宣称此前无实现；本功能落地后按实现结果将其修正为真
- 用户决策记录：本地缓存 / 预览+发布 / 只补缺失 / TTL 自动刷新 / 歧义跳过 / 后段全局唯一匹配
- 实现决议（code-review 回执）：整设 cost 时显式带 `cacheWrite: 0`（与既有落盘惯例一致，保发布后 pending 收敛）；reasoning 按字面“缺键即补”（含 false）；数值一律写 float64（JSON 域归一）；`handleGatewayPublish` 的 providers-wrapper 与单条目同一口径
- 实现决议（code-review 回执 2026-09-06 + 修复）：`handlePutGateway` 支持 providers wrapper（逐条目校验 + 补齐后整文件原子写，与发布路由同一口径，抽 `validateGatewayModels`/`writeGatewayFile` 共用）；存量 cost 缺 cacheWrite 补零归一但不计 enriched；数值判定接受全部 Go/JSON 数值类型；warning 携带上游状态码
