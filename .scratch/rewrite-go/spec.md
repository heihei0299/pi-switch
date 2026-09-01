# pi-switch Go 重写（参照 CLIProxyAPI 核心架构）

Status: ready-for-agent

## Problem Statement

pi-switch 现有 Rust + TypeScript 实现承担 pi 客户端的轻量 profile 切换与本地代理：管理多个供应商（Supplier）/上游（Upstream）、提供网关（Gateway）按模型名称路由、支持同模型 failover 与 OpenAI/Anthropic/Responses 互转、记录请求日志（Request Log）并提供统计窗口（Stats Window）聚合、通过 WebUI/TUI/CLI 暴露管理能力。但现有实现存在结构性约束：`proxy.rs` 单文件 224KB 扁平、依赖 `napi-rs` 绑定 Node.js 导致交叉编译与发布复杂度高（多平台 `.node` 预构建）、模块划分与 CLIProxyAPI 的 `internal/` 分层相比可维护性不足。需要一次架构级重写，以 Go 重新实现，参照 CLIProxyAPI 的模块化设计与 translator 复用，但仅取其核心代理能力，不引入插件生态/Home 集群/WebRTC 等正交设施，保持面向 pi 单用户、JSON 配置、SQLite 存储的现有定位。

## Solution

用 Go 重写 pi-switch，HTTP 框架选 `gin`，项目采用 CLIProxyAPI 式标准布局（`cmd`/`internal`/`pkg`/`webui`），编译为独立守护进程通过 HTTP API 与 pi 集成，不再依赖 `napi-rs`/CGO。配置保持 JSON 为主、per-request 读取、schema 兼容与旧字段自动迁移；认证首版仅 api-key 并预留 `AuthProvider` 扩展点；保留现有 React WebUI 仅适配 Go 后端（`embed.FS` 嵌入，管理 API 规范化为 `/api/*`）；代理核心直接复用 CLIProxyAPI 的 `translator` 包并按 `router/failover/convert/forward/limit/stream` 拆分，failover 保留逐候选尝试语义，contextWindow 限流与 SSE/WebSocket 流式沿用现有语义；统计层保留 SQLite + `requests.log`，Go 驱动选 `modernc.org/sqlite` 纯 Go，用量解析与查询导出 1:1 移植并保留 `sessionScan`；构建保持两阶段 `webui + go build`，多平台交叉编译后通过 npm 包装 Go 二进制发布（`@heihei0299/pi-switch`），`~/.pi-switch` 数据零改动兼容；TUI 用 `charmbracelet/bubbletea` 重写。

## User Stories

1. As a pi 用户, I want to 通过 `pi-switch use <name>` 切换当前供应商, so that pi 客户端的下一次请求走新 profile 的上游
2. As a pi 用户, I want to 通过 `pi-switch add/list/show/remove/duplicate` 管理供应商, so that 多个 api-key/多模型集合可被统一管理
3. As a pi 用户, I want to `config.json` 保留 JSON 格式且 per-request 热生效, so that 手改配置无需重启代理
4. As a pi 用户, I want to 旧版 `config.json`（含 `injectOpenCodeAttribution` 等遗留字段）自动迁移到 `conversationSource`, so that 升级后历史配置不丢失
5. As a pi 用户, I want to 网关按请求体中的模型名称路由到对应供应商（`providers[providerPrefix]` 合成视图经显式 `Gateway Publish` 落盘）, so that 不同模型可指向不同上游而共享单一本地入口
6. As a pi 用户, I want to 同一模型在首选供应商失败时按 `failover` 链逐候选重试, so that 单点故障不阻断请求
7. As a pi 用户, I want to OpenAI Chat Completions / Responses / Anthropic Messages 三种协议自动互转, so that pi 侧统一用一种协议即可访问不同上游
8. As a pi 用户, I want to 当 `api=openai-responses` 的供应商收到 `/v1/responses` 时走透传，否则走转换, so that 原生 Responses 能力不被降级
9. As a pi 用户, I want to 请求的 `max_output_tokens`/`max_tokens` 按 `ModelEntry.contextWindow/maxTokens` 与输入估算自动钳制（估算=序列化长度/4 向上取整，预留 4096，clamped = max(16, contextWindow-est-4096) 与 maxTokens 取最小）, so that 超窗请求不会以 400 溢出
10. As a pi 用户, I want to 流式（SSE）与非流式响应均被正确转发且用量（Usage）被 StreamTee 解析, so that 调用方拿到完整流且统计不丢
11. As a pi 用户, I want to 代理记录每条请求到 `requests.log`（时间、成败、供应商、模型、延迟、token 使用量、消费、会话标识）, so that 统计页可回溯
12. As a pi 用户, I want to 统计窗口支持当天/24小时/7天/自定义区间四档且作用于全部聚合, so that 我可按时间维度看 token/消费
13. As a pi 用户, I want to 按对话（Conversation，`x-conversation-id`/`x-opencode-session`/`conversation_id` 聚合，无标识归 `unlabeled`）查看 token 与消费, so that 单次对话成本可度量
14. As a pi 用户, I want to 在 WebUI 统计页看到总 token、缓存命中率、总消费（unknown 行显示 `-`）、按对话与按请求明细, so that 浏览器即可完成审计
15. As a pi 用户, I want to 导出请求日志为 JSON/CSV, so that 离线分析可行
16. As a pi 用户, I want to 通过浏览器打开 `http://127.0.0.1:43110` 的 WebUI 管理所有能力, so that 无需记忆 CLI 参数
17. As a pi 用户, I want to WebUI 与管理 API 共用 basic auth（非 loopback 绑定时生效，密码存 `~/.pi-switch/webui_password`）, so that 局域网暴露时有基础防护
18. As a pi 用户, I want to `pi-switch proxy start --daemon` 与 `webui start --daemon` 以 pid 文件管理后台进程（`~/.pi-switch/*.pid`，支持 stop/status）, so that 代理与面板可常驻
19. As a pi 用户, I want to `pi-switch tui` 在终端内完成 profile 列表/切换、网关状态、统计简览, so that 无浏览器时也可操作
20. As a pi 用户, I want to 继续 `npm install -g @heihei0299/pi-switch` 安装且 `~/.pi-switch` 下历史数据零改动兼容, so that 升级无感知
21. As a pi 用户, I want to 模型目录（Model Catalog, models.dev）作为模型元数据权威源 enrich 本地 `ModelEntry`（`cost/limit/reasoning/modalities` 按分字段策略合并）, so that 模型参数无需手填
22. As a pi 用户, I want to 供应商可配置多上游（`Upstream[]`，含 `baseUrl/apiKey/headers/weight/name`），网关当前仅取首条主上游, so that 多节点供应商可被表达
23. As a pi 用户, I want to `package` 管理（`add/install/uninstall/enable/disable/sync`）在 Go 重写后保持可用, so that pi 扩展/技能/主题的生命周期不受重写影响
24. As a pi 用户, I want to `cc-switch` 导入（`listCcsSwitchProviders`/`importCcsSwitchProviders`）保持可用, so that 存量用户可迁移
25. As a pi 用户, I want to 请求头 `x-conversation-name` 的非 Latin1 字符 percent-encode 与代理侧对称解码后在 WebUI 可读, so that 中文会话名不乱码
26. As a pi 运维, I want to `go build` 交叉编译矩阵覆盖 `linux/darwin/windows × amd64/arm64` 且无需 CGO, so that 单一源码可产出全平台制品
27. As a pi 开发者, I want to `npm run build:webui && go build -ldflags "-s -w"` 两阶段构建且 `embed.FS` 嵌入 `webui/dist`, so that 单一 Go 二进制即含前端

## Implementation Decisions

- **框架与布局**：HTTP 选 `gin`（与 CLIProxyAPI 一致，生态与中间件丰富）；项目采用标准 Go 布局 `cmd/pi-switch` 入口、`internal/{api,proxy,config,auth,stats,webui,tui,package,gateway,database}`、`pkg` 可复用包、`webui` 保留 React 前端；不引入 CLIProxyAPI 插件生态。
- **与 pi 集成**：Go 编译为独立守护进程，pi 通过 HTTP 代理（`127.0.0.1:43112` 默认）与本地网关通信；`bin/pi-switch.js` 按平台分发执行对应 Go 二进制，保留 npm 安装路径；不再依赖 `napi-rs`/CGO。
- **配置系统**：主格式 JSON，默认路径 `~/.pi-switch/config.json`，Go 支持兼容读取 YAML 但默认写回 JSON；采用 per-request 读取（每次请求重载文件），保留现有热生效语义；`PiSwitchConfig` 保持 `version/current/profiles/settings` 结构，`ProviderProfile`/`ModelEntry`/`Upstream`/`Settings` 字段名与现有 JSON 兼容；旧字段 `injectOpenCodeAttribution` 自动映射到 `conversationSource`（`true→proxy`/`false→off`），`version<2` 升到 2；校验采用 struct tag + 自定义校验，加载时收集错误列表统一返回。
- **设置默认值**：`providerPrefix` 默认 `pi-switch`、`writeMode` 默认 `gateway`、`gatewayApi` 默认 `openai-completions`、`proxy.host` 默认 `127.0.0.1`/`port` 默认 `43112`、`web.host` 默认 `127.0.0.1`/`port` 默认 `43110`、`conversationSource` 默认 `sessionScan`；`ModelEntry` 默认 `input=["text"]`、`contextWindow=128000`、`maxTokens=16384`。
- **认证架构**：首版仅 `api-key` 转发（`profile.api_key`/`upstreams[].api_key`/`headers`），Go 中抽象 `AuthProvider` 接口预留 OAuth 扩展点但不实现具体流程；不引入 `auth-dir` 多账户存储与客户端 `api-keys` 列表；管理 API/WebUI 沿用 basic auth。
- **WebUI 与管理 API**：保留现有 React + vite 前端，Go 用 `embed.FS` 嵌入 `webui/dist`；管理 API 保留现有 REST 端点并规范化为 `/api/*` 前缀（`profiles` CRUD、`gateway publish`、`stats` 查询、`export`、`package`、`daemon` 控制等），前端仅改 `baseURL`；认证复用 basic auth。
- **代理核心拆分**：`internal/proxy` 拆为 `router`（路由分发）、`failover`（逐候选尝试链）、`convert`（封装 translator 调用）、`forward`（HTTP 转发含 User-Agent）、`limit`（contextWindow 估算与 `max_output_tokens`/`max_tokens` 钳制）、`stream`（SSE/WebSocket 转发与用量解析）；`proxy.rs` 224KB 巨文件不再保留。
- **协议转换**：直接复用 CLIProxyAPI 的 `internal/translator`/`sdk/translator` 包，适配 pi-switch 的网关按模型名称路由语义；按需裁剪 Gemini 相关依赖，保留 OpenAI Chat Completions / Responses / Anthropic Messages 核心路径；`responsesMode` 语义保留 `auto/passthrough/convert`（`auto` 下 `openai-responses→passthrough`、`openai-completions→convert`），`passthrough` 仅允许 `openai-responses`，`convert` 仅允许 `openai-completions`，不兼容组合在保存时阻止；失败时按透传返回上游错误，不自动降级。
- **限流与流式**：输入估算 `ceil(jsonLen/4)` 涵盖 `input+tools/instructions` 或 `messages+tools`，`available = contextWindow - est - 4096`，`clamped = max(16, available)` 再与 `model.maxTokens` 取最小后重写；流式通过 StreamTee 在客户端与用量解析间分流，`SseUsageParser` 解析 `usage` 事件。
- **统计与存储**：保留 SQLite + `requests.log`（JSONL 追加）结构，Go 驱动 `modernc.org/sqlite` 纯 Go 无 CGO；用量解析 `StreamTee`/`SseUsageParser` 1:1 移植到 Go；查询保持现有聚合口径（仅成功且非 retry 且解析到 usage 的行参与求和），支持 `totalCost`/`costUnknown` 与对话/请求维度；导出 `export_logs_json`/`export_logs_csv` 追加消费列；`sessionScan` 保留（离线扫描 `~/.pi/agent/sessions` JSONL，按 `model+时间窗口±2s` 关联 `requests.log`，`prompt_tokens` 二次校验，查询时 join，不重写日志），`settings.conversationSource` 枚举 `proxy|sessionScan|off` 默认 `sessionScan`，轮询 3s，`--no-session` 幽灵会话天然排除。
- **构建与发布**：两阶段 `npm run build:webui && go build -ldflags "-s -w"`；Go 交叉编译矩阵 `GOOS=linux/darwin/windows × GOARCH=amd64/arm64`，因 `modernc.org/sqlite` 纯 Go 无需 CGO；发布继续以 npm 包 `@heihei0299/pi-switch` 为主（包内含多平台 Go 二进制，`bin/pi-switch.js` 按平台选择），GitHub Releases 二进制作为补充可选；`~/.pi-switch` 数据（`config.json`、SQLite DB、`webui_password`、备份）零改动兼容。
- **TUI**：用 `charmbracelet/bubbletea` + `bubbles` + `lipgloss` 重写现有 `ratatui`+`crossterm` 实现，作为 `pi-switch tui` 子命令独立运行，功能 1:1（profile 列表/切换、网关状态、统计简览）。
- **其它能力保留**：`package` 管理（`catalog/package/package_ops`）、`cc-switch` 导入、`presets`/`sync`/`scan_pi`/`credits` 等能力在 Go 中保持可用；多上游 `Upstream` 聚合保留，网关当前仅取首条主上游。

## Testing Decisions

- **好的测试**：只测外部行为——给定 CLI 输入/HTTP 请求/文件系统状态，断言 CLI 退出码与输出、HTTP 响应（含流式事件）、`config.json`/`requests.log`/SQLite 的落盘结果；不测 `internal/*` 内部函数与私有状态。
- **缝（Seam）**：单一最高缝——Go 二进制的外部行为（CLI + `127.0.0.1:43112` 代理 + `127.0.0.1:43110` 管理/WebUI + 文件系统 `~/.pi-switch/*` + `~/.pi/agent/sessions`）。该缝覆盖全部 8 个决策的外部可观测行为，符合“最少缝、最高缝”原则。
- **代理网关**（先例：现有 `proxy` 流式与限流单测、`cargo test --lib`）：以 mock 上游（`httptest.Server`）为上游，测模型路由、failover 逐候选重试、协议互转（Chat/Responses/Anthropic）、`max_output_tokens`/`max_tokens` 钳制、`User-Agent` disguise、SSE/WebSocket 流式与 `SseUsageParser` 计量。
- **配置与管理 API**（先例：`config` 校验单测、`service` 侧 profile CRUD）：测 `config.json` per-request 读取与热生效、旧字段迁移、schema 校验、` /api/*` 的 profile/gateway/校验/导入等端点、basic auth。
- **统计与存储**（先例：`stats` 聚合与旧行兼容、`export` 列）：测 `requests.log` 追加、`totalCost`/`costUnknown` 聚合、窗口过滤、对话归因（`sessionScan` 关联）、`export_logs_json/csv` 列、旧行无 `cost` 字段兼容。
- **WebUI/TUI/守护进程**（先例：`webui` vitest `StatsPanel`/`formatCost`、`TUI` 渲染）：测 `embed.FS` 前端可访问性、管理 API 与前端联动、TUI `pi-switch tui` 渲染与交互、daemon `start --daemon/stop/status` 与 pid 文件生命周期。
- **工具**：Go `go test ./...` 集成测试为主；WebUI 侧 `NODE_ENV=test npx vitest run`（`NODE_ENV=production` 下 `act` 报错）；保留现有 `extensions/**/*.test.ts` 模式作为扩展测试参考。

## Out of Scope

- 不替换 pi 客户端本身，仅替换 pi-switch 代理层
- 不重设计 `opencode-go` 上游协议，保持兼容
- 不引入 CLIProxyAPI 的 Home/集群模式、WebRTC、Redis Queue、Pion 等通信基础设施
- 不引入 CLIProxyAPI 插件生态（动态库/命令行插件、store 扩展）
- 首版不提供可嵌入 Go SDK，仅独立守护进程 + HTTP API
- 首版不实现多供应商 OAuth（Codex/Claude/Grok 等），仅 `api-key` + 预留 `AuthProvider` 扩展点
- 不引入客户端多 `api-keys` 列表认证（单用户本地代理无需）
- 不改变 `~/.pi-switch` 数据目录语义与现有备份机制

## Further Notes

- 术语以 `CONTEXT.md` 为准：Token 使用量（prompt/completion/cached/reasoning 四部分）、消费（Cost，`$ / 1M tokens` 定格写入）、统计窗口、缓存命中率、对话/未标记、请求日志/请求明细、模型目录/模型元数据、供应商/上游/网关/网关发布、Responses 透传模式。
- 约束：保留包名 `@heihei0299/pi-switch` 与 npm 发布线；`config.json` 字段名与结构兼容，旧用户零迁移成本；热重载为 per-request 读取，无 `fsnotify` 依赖。
- 与 CLIProxyAPI 的关系：参照其 `gin` + `internal/` 分层与 `translator` 复用，裁剪 Gemini/Home/插件等正交能力；`modernc.org/sqlite` 保证纯 Go 交叉编译。
- 参考地图：`.scratch/rewrite-go/map.md` 8 项决策；子票 `issues/01-go-framework.md` 至 `08-tui.md` 已全部 `resolved`。
