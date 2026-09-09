# 供应商余量查询面板（opencode-go 首发，预留 codex）

Status: ready-for-agent

## Problem Statement
供应商列表当前仅展示静态配置（api/baseUrl/模型数/暴露数），用户无法在不离开 pi-switch 的情况下获知 opencode-go 订阅的剩余额度、已用与总额、过期时间；需频繁到上游控制台查询。中期需同形态支持 codex 订阅，但两者上游接口路径与响应字段不同，需一次抽象避免后续重构。

## Solution
在供应商（`ProfilesPanel`）的每张卡片上内联余量小面板，仅对命中 opencode-go 的供应商自动显示；通过 pi-switch 后端代理 `GET /api/profiles/:name/credits` 转调上游 `GET {baseUrl}/v1/credits`（Bearer apiKey），后端归一化为前端固定结构后返回。卡片挂载时自动查询一次并提供手动刷新，失败仅卡片内联提示与重试，不影响供应商 CRUD 与网关。

## User Stories
1. As an opencode-go 用户, I want to 在供应商卡片上直接看到余量摘要, so that 无需跳转控制台即知是否需充值
2. As an 管理员, I want to 卡片挂载时自动查询且可手动刷新, so that 余量保持新鲜且可控
3. As an 管理员, I want to 看到余额/已用/总额/过期四字段与进度条, so that 直观判断消耗与到期风险
4. As an 管理员, I want to 查询失败时在卡片内看到错误与重试, so that 单个供应商失败不影响其他供应商与网关
5. As an 管理员, I want to 多上游供应商仅查询主上游余量, so that 口径与网关聚合一致且不扇出多次查询
6. As an 管理员, I want to 后续接入 codex 订阅时面板无需改造, so that 扩展仅新增后端 fetcher
7. As an 访客, I want to 非 opencode-go 供应商卡片不显示余量入口, so that 界面保持简洁

## Implementation Decisions
- 显示条件：前端按供应商主上游 `baseUrl` 是否包含 `opencode.ai`（或 `api` 标识）判定是否渲染余量区；codex 后续新增命中分支，面板组件不分支
- 链路与安全：前端不直调上游；新增 `GET /api/profiles/:name/credits`（profiles Router），后端用该供应商 `resolvedUpstreams()[0]` 的 `baseUrl + /v1/credits` + `Authorization: Bearer {apiKey}` 转调，超时 5s，错误隔离（400/401/429/5xx 映射为归一化错误，不影响其他路由）
- 归一化契约：后端将 opencode-go 与未来 codex 的原始响应统一映射为 `{ balance: number, used: number, total: number, remaining: number, percent: number, resetAt/expiry: string|null, raw: unknown }`，前端仅依赖归一化字段；`raw` 保留原始体用于调试，不参与渲染权威
- 卡片交互：卡片内新增余量区（余额/总额 + 进度条、小字已用与过期、刷新按钮）；`useEffect` 挂载时自动 fetch，刷新按钮重调；loading 用小 spinner，错误用红字 + 重试按钮；无后端缓存 v1
- 多上游：仅查询 `resolvedUpstreams()[0]`，界面小字注明“主上游”；不为每条 upstream 扇出查询
- 扩展抽象：后端新增 `src-rust/credits.rs`（或 `gateway.rs` 旁独立模块），定义 `CreditsFetcher` trait/注册表，按供应商特征路由到 `OpencodeGoFetcher` / 未来 `CodexFetcher`；新增订阅仅新增实现文件与注册一行

## Testing Decisions
- 好测试只验外部行为：`GET /api/profiles/:name/credits` 对 opencode-go 供应商返回归一化 JSON、非命中供应商返回 404/不支持、401/超时返回归一化错误且不影响 `GET /api/state` 与 `GET /api/models/gateway/preview`；前端卡片挂载时自动调一次 credits、手动刷新重调、失败时卡片内联错误与重试可点击；不测内部 fetcher 调用计数
- 测试 seam：Web API 集成（credits 代理归一化与错误隔离）、credits 模块单元（opencode-go 映射、codex 占位、主上游选择）、WebUI 组件单测（卡片内联余量区渲染/刷新/错误重试、非命中不渲染）
- 先例复用：沿用既有 `web::make_profiles_router` 与 `make_gateway_router` 隔离形态、gateway 健康/预览的错误隔离测试形态、ProfilesPanel 的供应商卡片单测形态

## Out of Scope
- 不引入定时轮询或后端缓存（v1 仅按需查询）
- 不改变供应商 CRUD、网关发布、代理转发路径
- 不在 GatewayPanel 或独立 Tab 展示余量
- 不前端直调上游，不在浏览器暴露 apiKey

## Further Notes
- 约束来源：CONTEXT.md 术语（供应商/上游/网关）、ADR-0006 隔离原则（供应商与网关错误隔离）、本次 grilling 共识（opencode-go 首发、codex 预留、卡片内联、后端代理、归一化、主上游、挂载自动+手动刷新、内联错误重试）
- 后续 codex 扩展路径：新增 `CodexFetcher`（路径/鉴权/字段映射不同）并注册到 `credits` 注册表，前端面板零改动
