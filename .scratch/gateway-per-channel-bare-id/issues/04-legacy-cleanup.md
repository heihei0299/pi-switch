# 04: 旧形态全量清理

**What to build:** 彻底移除单 provider 与斜杠兼容形态，存量旧 `models.json` 与旧前缀请求视为脏数据直接废弃，代码与文档与 `CONTEXT.md/ADR-0009` 一致，全量测试绿。

**Blocked by:** 03: 斜杠硬拒绝与 WebUI 分组落地

**Status:** ready-for-agent

- [ ] 删 `internal/config: Settings.GatewayAPI/ProviderPrefix` 与 `Supplier` 顶层 `models/exposedModels` 回退分支，`Channel.api` 必填校验
- [ ] 删 `internal/gateway` 与 `internal/server` 中旧 `id` 含 `/` 的剥离去重与单 provider 回退分支
- [ ] `docs/adr/0009` 与 `CONTEXT.md` 已对齐，`gateway_test/config_channel_api_test/server_test` 旧前缀用例删除
- [ ] `go test ./...` 与 `NODE_ENV=test npx vitest run` 全绿，`npm run build:webui && go build` 后浏览器手工验证多 provider 裸 `id` 与代理三分支
