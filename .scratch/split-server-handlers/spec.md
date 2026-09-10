# Spec: 拆分 server handler 并收敛重复实现

**Status**: ready-for-agent
**Baseline**: commit `46d8153` ｜ **Date**: 2026-09-10
**Slug**: `split-server-handlers`
**Scope**: `internal/server/server.go` 文件拆分、`internal/config` 读写收敛、`internal/proxy` 成本单一实现、`cmd/pi-switch` 与 `internal/tui` 调用点、`docs/`（不含新功能与行为契约变更）

## Problem Statement

维护者面对一个 3645 行、118 个符号的 `internal/server/server.go`：供应商管理、网关、代理核心、统计、包管理、设置六类 HTTP handler 和路由、认证、静态资源、共享 helper 全混在一起。最近 50 个提交中有 20 次改动该文件，任意领域的改动都要在大文件里定位、评审，变更碰撞面持续放大。同时拆分调查暴露了三处真实重复：config 路径解析 4 份实现、config 写盘 3 份实现（且只有一处应用保存迁移）、成本计算 2 份实现（其中 `internal/proxy` 里那份零调用者）。文件大不是问题本身，**领域耦合与重复实现才是**。

## Solution

把 `internal/server` **同 package 拆成 6 个域文件 + 残余 kernel**，并收敛已证实的重复：

- 纯文件移动（一个提交，可用 `--color-moved` 验证零逻辑改动），测试文件一个不搬；
- 被 2+ 域调用的 helper 显式留在 kernel（`server.go`），域文件只依赖 kernel + 自身；
- config 路径解析与原子写归还 `internal/config`，三处调用统一；
- 成本计算只留 `internal/proxy.CalcCost` 一份（带测试），删除 server 内副本；
- 删除零引用死代码；
- 文档同步到新文件与符号引用。

路由注册、认证、静态资源、构建信息与既有 API 契约完全不动。

## User Stories

1. 作为维护者，我想改网关 API 时只碰 `gateway_handlers.go`，以便评审面收窄到 250 行量级。
2. 作为维护者，我想改代理流式/非流式逻辑时 diff 不混入统计与包管理代码，以便回归定位更快。
3. 作为变更作者，我想代理核心的改动只在一个文件内解决，以便减少与供应商管理改动的 merge 冲突。
4. 作为 reviewer，我想第一个提交能用 `--color-moved` 确认为纯移动，以便把注意力只放在第二个收敛提交。
5. 作为维护者，我想任何被 2+ 域使用的 helper 都显式位于 kernel，以便共享面一眼可见。
6. 作为新读者/agent，我想从文件名直接定位 handler 所属领域，以便不再靠行号 grep 导航。
7. 作为 CLI 用户，我想 `provider use/delete` 保存配置时应用与 WebUI 相同的保存迁移，以便写回的文件不会残留旧字段。
8. 作为 TUI 用户，我想切换 profile 的保存行为与 WebUI 一致，以便同一份配置不因入口不同而漂移。
9. 作为维护者，我想 config 路径解析（env + 默认 home）只有一份实现，以便新增入口不会漏掉 `PI_SWITCH_CONFIG`。
10. 作为维护者，我想 config 原子写盘（临时文件 + rename）只有一份实现，以便不会漏掉迁移或原子性。
11. 作为维护者，我想成本计算只有一份实现且被真实调用，以便 money path 不出现行为分叉。
12. 作为维护者，我想成本逻辑的既有测试继续守护唯一实现，以便负值/nil/常规三种边界不回退。
13. 作为维护者，我想删除零调用者的 `contains` 与 `modelsDevCatalog`，以便减少误读与误改。
14. 作为未来实现者，我想 `internal/proxy` 有真实调用者，以便后续抽 proxy core 时落点已存在。
15. 作为维护者，我想 `server.go` 只保留路由注册、认证、静态资源、构建信息与 kernel，以便残余内容有明确清单。
16. 作为 reviewer，我想验收条件可机械执行（测试全绿 + 残余内容审计 + color-moved），以便"完成"没有解释空间。
17. 作为维护者，我想文档在拆分后仍指向正确文件与符号，以便导航不失效。
18. 作为维护者，我想 retry.go 的休眠原语不被本拆分激活或删除，以便 per-conversation 熔断仍可按既有 spec 复用。
19. 作为维护者，我想本次拆分不新增 ADR、不改领域词汇表，以便决策成本与改动风险匹配。
20. 作为 agent，我想移动提交与收敛提交分开，以便"移动"永远可独立验证与回退。

## Implementation Decisions

**D1 包策略**：同 `internal/server` package 拆文件，不划子包。理由：handler 通过 30+ 处未导出 helper 耦合（`configPath`、`saveConfig`、`validateProviderProfile`、`ensureMutationChannel` 等），子包化需导出符号、迁移测试、重排 import，另立 spec。

**D2 目标文件与映射**（`*_handlers.go` 命名；`retry.go`/`outbound.go`/`legacy_log.go` 不动）：

| 文件 | 内容（符号归属） | 约行数 |
|---|---|---|
| `proxy_handlers.go` | `handleModels`、`resolveRoute`、`pinChannelAttempts`、`conversationIDFrom`、`findModelEntry`、`incomingProtocol`、`estimateEffectiveLen`、`clampBody`、`findFrameEnd`、`extractData`、`handleChatCompletions`、`handleStream`、`streamPassthrough`、`streamConvert`、`cloneMap`、`extractUsage`、`logRequest`、`dump400`、`handleDumps` | ~875 |
| `profile_handlers.go` | `handleGetConfig`、`handlePutConfig`、`handleGetState`、profiles CRUD/duplicate/test、`validateProfileResponsesMode`、`truncateForTest`、fetch-models 链路（`handleFetchModelsForChannel`、`fetchUpstreamUsage`、`fetchUpstreamIDs`、`handleFetchModels`、`enrichModelsWithCatalog`）、`handlePutModels`、`handlePutExpose`、`handlePutSpoof`、`handleGetCredits`、`handlePresets`、`handlePresetDetail`、`handleDoctor`、`validateResponsesMode`、`isValidChannelName`、`ensureMutationChannel`、`channelIndex`、`validateProviderProfile`、`handleValidate`、`handleCcsProviders`、`handleCcsImport` | ~1065 |
| `gateway_handlers.go` | `handleGetGateway`、`gatewayPreviewRequest`、`handleGatewayPreview`、`handleGatewayPreviewPost`、`serveGatewayPreview`、`buildGatewayPlan`、`enrichProposedModels`、`handlePutGateway`、`handleGatewayHealth`、`handleGatewayStart`、`handleGatewayPublish` | ~256 |
| `package_handlers.go` | `piSwitchDBPath`、`openPiSwitchDB`、`ensurePiPackageColumn`、`piAgentSettingsPath`、`parsePackageSpec`、`ListInstalledPackages`、`ImportPiPackages`、`boolInt`、全部 `handlePackage*` | ~379 |
| `settings_handlers.go` | `handleBackups`、`handleProxyStatus`、`handleWebUIInfo`、`handleBuildInfo`、`handleInit`、`handleProxyStart/Stop`、`handlePutFailover`、`handleGet/PutSettings`、config export/import/restore 三个 stub | ~150 |
| `stats_handlers.go` | `normalizeRange`、`window`、`parseWindowQuery`、`statsWindowFor`、`statsPageLimit`、`newStatsService`、`handleStats`、`handleStatsConversations`、`handleConversationRequests`、`handleLogsExport`、`sessionScanCandidates`、`conversationCandidates`、`effectiveConversationID` | ~408 |
| `server.go`（kernel） | 路由注册、auth（`authMiddleware`/`isLoopback`/`splitHostPort`/`webUIPasswordPath`/`resolveWebUIPassword`）、静态资源、构建信息与 embed vars、`configPath`、`saveConfig`、`resolveModelsDevProvider` | ~490 |

**D3 kernel 规则**：被 2+ 域调用的 helper 必须位于 `server.go`；域文件不得调用其他域文件的内部 helper。`resolveModelsDevProvider` 因被 profile 与 gateway 共用，留在 kernel。不在本轮加机械守护测试（无证据不加机制），漂移出现再加。

**D4 测试文件不搬迁**：45 个 `internal/server/*_test.go` 原地不动；纯移动不产生测试 diff。

**D5 config 读写收敛（行为修正）**：`internal/config` 新增 `ResolvePath()`（`PI_SWITCH_CONFIG` > `~/.pi-switch/config.json` > `/tmp` 回退）与 `SaveAtPath(cfg, path)`（内部调用 `MigratedForSave` + 临时文件 rename）。server/tui/cmd 三处路径与写盘实现改调用新函数并删除旧实现。**行为改变被接受**：TUI 与 CLI 保存时从此与 server 一样应用保存迁移。

**D6 cost 单一实现**：`internal/proxy.CalcCost` 改为指针入参（nil entry 返回 nil），成为唯一实现；`proxy_handlers.go` 6 个调用点直接调用；删除 `server.computeCost`；`internal/proxy/cost_test.go` 随签名更新。

**D7 死代码删除**：`contains`、`modelsDevCatalog` 已确认全仓（含测试）零引用，删除。`dump400` 同样零调用者，但它与 `handleDumps` 端点配对，删除生产者会造成链路不完整，故保留原样并在 Further Notes 记录为后续候选。

**D8 提交切分与交付**：commit 1 纯文件移动（零逻辑改动）；commit 2 收敛（D5–D7 + 文档）。一次交付，两个提交。

**D9 文档同步**：`docs/architecture.md` 入口表与调用链的文件路径改为新文件；`docs/architecture-review.md` 将 A3 标记完成、把行号类证据改为符号引用。`.scratch` 历史 spec 不改。

**D10 明确非目标**：不引入 service 接口层、不建子包、不改任何路由/JSON 契约、不激活 `retry.go`、不搭车 architecture-review 的 A1/A2/A4/A5/A6。

## Testing Decisions

- **seam 全部复用既有**：HTTP router（全部 handler 的行为面）、`internal/config` 包函数、`internal/proxy.CalcCost`。本拆分不新增 seam。
- **好测试的定义**：只验证外部行为——API 路由与响应、config 保存产物、纯函数边界；不测文件布局、不测函数位于哪个文件。
- **移动提交**：不需要新测试；既有测试全绿即回归证明（测试文件零 diff）。
- **新增测试**：`internal/config` 一个测试验证 `SaveAtPath` 应用迁移、原子写、生成合法 config shape；沿用 `save_migration_test.go` 的既有风格。
- **更新测试**：`internal/proxy/cost_test.go` 随指针签名调整，保留 nil / 负值 / 常规三个 case。TUI 测试中 `saveConfig(cfg, path)` 调用点改为 `config.SaveAtPath`。
- **验收门禁**（执行阶段需用户明确授权编译/测试命令）：`go test ./...` 全绿；`git diff --color-moved` 确认 commit 1 全部为移动；`grep` 审计确认 `server.go` 只含 D2 清单符号；`gofmt -l` 无输出。
- **prior art**：`internal/config/save_migration_test.go`、`internal/proxy/cost_test.go`、`internal/server/api_contract_test.go`。

## Out of Scope

- architecture-review 的 A1（认证缺口）、A2（CLI 假成功）、A4（retry 休眠注释）、A5（遗留 JS 归属）、A6（config 读缓存）；
- L3 跨包迁移：把 package/CCS 域抽到 `internal/piagent`，需要动测试包归属与 piagent 接口，另立 spec；
- 任何 handler 逻辑、路由表、JSON 契约、WebUI 变更；
- 子包化、service 接口层、结构守护测试；
- 激活或删除 `retry.go` 及其配置字段；
- 删除 `/api/dumps` 端点；
- 回改 `.scratch` 历史文档。

## Further Notes

- 映射表基于 commit `46d8153` 的符号清单；若实现时发现清单漂移，以"域归属 + kernel 规则"为准，不在 spec 外扩大删除范围。
- `profile_handlers.go` 约 1065 行，是本轮最大文件；不在本轮继续细分。触发条件：域文件 >1200 行或出现跨域 helper 漂移，再评估拆分或机械约束。
- dump 链路（`dump400` + `/api/dumps`）：生产者零调用者，端点保留但永远空目录；作为独立清理候选记录。
- 预期收益：`server.go` 从 3645 行降至 ~490 行；任意单域改动集中在一个 ≤1100 行文件；config/cost 重复归零；测试文件零改动通过。
- 本 spec 不产生 ADR（可逆、不反直觉、无长期权衡）；不修改 `CONTEXT.md`（拆分是实现结构，非领域词汇）。
