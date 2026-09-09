# 01 — 后端余量代理与归一化抽象

**What to build:** 后端新增 `src-rust/credits.rs` 抽象与 `GET /api/profiles/:name/credits` 代理，opencode-go 首发归一化，后续 codex 仅新增 fetcher。

**Blocked by:** None — can start immediately

**Status:** resolved

- [x] `GET /api/profiles/:name/credits` 对 opencode-go 供应商（baseUrl 含 opencode.ai）返回归一化 JSON `{ balance, used, total, remaining, percent, resetAt/expiry, raw }`
- [x] 非命中供应商请求返回 404/不支持错误，错误隔离不影响 `GET /api/state` 与网关预览/健康
- [x] 上游 401/超时(5s)/5xx 返回归一化错误，错误隔离不影响供应商 CRUD 与网关，且不写盘
- [x] 多上游供应商仅查询主上游 `resolvedUpstreams()[0]` 的 `/v1/credits`，不扇出
- [x] 抽象 `CreditsFetcher` trait/注册表，OpencodeGoFetcher 独立实现，后续 codex 仅新增文件与注册

## 实施总结
- 提交：`8203b4d` — `feat(credits): 后端余量代理与归一化 (#01)` / `d3f5024` — `docs: align README with credits proxy (#01)`
- 实现的 seams：`GET /api/profiles/:name/credits` 归一化返回（balance/used/total/remaining/percent/resetAt/expiry/raw） / 非命中 404 隔离 / 上游 401/5xx/超时(5s)归一化错误隔离不写盘 / 多上游仅查主上游resolvedUpstreams()[0] / CreditsFetcher trait+注册表+OpencodeGoFetcher
- 验收标准：
  - [x] GET /api/profiles/:name/credits 对 opencode-go 供应商返回归一化 JSON
  - [x] 非命中供应商返回 404/不支持，隔离不影响 GET /api/state 与网关预览/健康
  - [x] 上游 401/超时(5s)/5xx 返回归一化错误，隔离不影响供应商 CRUD 与网关，且不写盘
  - [x] 多上游仅查询主上游 resolvedUpstreams()[0] 的 /v1/credits，不扇出
  - [x] 抽象 CreditsFetcher trait/注册表，OpencodeGoFetcher 独立实现，后续 codex 仅新增文件与注册
- 测试结果：318 项，全绿（cargo test --lib --test-threads=1）
- typecheck：通过（cargo check 0 errors）
- 文档对齐：已更新 README.md / README_ZH.md 架构列表（新增 credits.rs）
- 遗留 / 后续建议：CodexFetcher 预留扩展点已验证，后续仅新增实现文件与 registry 注册一行；前端 02 面板可直接依赖本接口
