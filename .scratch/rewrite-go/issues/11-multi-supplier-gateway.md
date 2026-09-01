# 11: 多供应商与网关发布（Upstream 聚合 + failover + sessionScan）

**What to build:** 扩展 10 的单供应商为多 `Supplier`/`Upstream[]`：`config.json:profiles` 支持 `upstreams` 数组（`baseUrl/apiKey/headers/weight/name`），网关按请求体 `model` 名称路由到对应 `Supplier`（合成视图 `providers[providerPrefix]` 经 `PUT /api/gateway/publish` 显式落盘到 `models.json`，发布外变更不自动写盘），同一模型在首选失败时按 `settings.proxy.failover` 链逐候选重试（保留现有语义，不引熔断），`settings.conversationSource` 三档（`proxy/sessionScan/off`，默认 `sessionScan`）控制 `x-conversation-id`/`x-opencode-session`/`conversation_id` 聚合，无标识记 `unlabeled`，`sessionScan` 复用 `~/.pi/agent/sessions` JSONL 离线关联（`model+时间窗口±2s`，`prompt_tokens` 二次校验，查询时 join，不重写 `requests.log`，`--no-session` 幽灵排除）。

**Blocked by:** 10 单供应商代理闭环

**Status:** ready-for-agent

- [ ] `profiles` 的 `upstreams` 非空时生效，网关当前仅取首条主 `Upstream` 的 `baseUrl/apiKey/headers` 转发
- [ ] `POST /v1/chat/completions` 的 `model` 路由到正确 `Supplier`，`PUT /api/gateway/publish` 显式写 `models.json:providers[providerPrefix]`，写前与写后 `models.json` 差异可断言
- [ ] 同模型 failover：首选 `Upstream` 返回 5xx 时按 `failover` 链逐个尝试，最终成功或全败返回末次错误；链可经 `PUT /api/config` 热更新
- [ ] `conversationSource` 三档：`proxy` 仅信请求头/`sessionScan` 走离线扫描/`off` 全记 `unlabeled`；`GET /api/stats?groupBy=conversation` 按 `conversation_id` 聚合正确
